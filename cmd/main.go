package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/antoni-ostrowski/op-reviewer/internal/config"
	"github.com/antoni-ostrowski/op-reviewer/internal/utils"
)

func main() {
	conf, err := config.New()
	if err != nil {
		slog.Error("failed to configure app", "error", err)
		os.Exit(1)
	}

	slog.Info("sucessfully created config", "details", conf)

	utils.ExecCmdPiped(exec.Command("ls", "-al"))
	utils.ExecCmdPiped(exec.Command("ls", "-al", conf.SourceCodePath))

	prompt := `
UNDER ANY CIRCUMSTANCES DO NOT READ OS ENVS DIRECTLY via env/printenv/cat /proc/self/environ or by reading files that contain secrets. Use only placeholders.

You are a code review agent. You are running inside the repo (cwd is repo root) already checked out at ${CI_COMMIT_SHA}. Do not clone.

1. Find changes: run git diff HEAD~1, git show --stat, git log -1 --name-only to get changed files and diff. Review only the diff, but use the repo to gain context about the change if needed/unclear.

2. Review for: bugs, logic errors, security issues, error handling, code quality, performance.

3. Return ONLY a single JSON string matching schema {"message":"<gh commands joined by \\n>"}. No markdown, no explanation, no extra keys. Put all gh commands inside the message field.

4. Use ONLY these bash placeholders with ${VAR} syntax:
- ${CI_COMMIT_SHA} - commit SHA to review
- ${CI_REPO} - owner/repo (e.g. octocat/hello-world) — use this for gh api paths
- ${CI_COMMIT_PULL_REQUEST} - pull request number
- gh CLI is already authenticated via OP_REVIEWER_GH_TOKEN, do NOT prefix commands with GH_TOKEN and do NOT handle auth, do not try to execute these gh cli commands, only respond with them in that correct JSON form.

 5. Generate gh commands to publish review:
- One summary review: gh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/reviews -f event="COMMENT" -f body="..." -f commit_id="${CI_COMMIT_SHA}"
- Zero or more inline comments: gh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/comments -f body="..." -f commit_id="${CI_COMMIT_SHA}" -f path="path/to/file" -F position=N
  position is NOT file line number. To get position, run: gh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/files --jq '.[].patch' or git diff HEAD~1, then count: position 1 is first line after first @@ header, position 2 is next, etc., through all hunks until next file. Use -F for position (integer).
Join all commands with \\n inside message.

Example message value:
"gh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/comments -f body=\"Avoid sync read in handler at src.js:12\" -f commit_id=\"${CI_COMMIT_SHA}\" -f path=\"src.js\" -F position=6\\ngh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/reviews -f event=\"COMMENT\" -f body=\"Overall: fix error handling, otherwise LGTM\" -f commit_id=\"${CI_COMMIT_SHA}\""	`

	cmd := exec.Command("opencode", "run", "--format", "json", "--model", conf.Model, "--log-level", "DEBUG", "--print-logs", strings.TrimSpace(prompt))
	fmt.Printf("cmd %v\n", cmd)
	cmd.Env = safeEnv(conf.AiApiKey)
	cmd.Dir = "./source"
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("res: %v\n", string(out))
		slog.Error("agent run failed", "error", err, "output", string(out))
		if apiErr := parseOpenCodeError(out); apiErr != nil {
			slog.Error("opencode api error",
				"error_name", apiErr.Error.Name,
				"error_message", apiErr.Error.Data.Message,
				"error_ref", apiErr.Error.Data.Ref,
				"session_id", apiErr.SessionID,
				"timestamp", apiErr.Timestamp,
			)
		}
		os.Exit(1)
	}

	fmt.Printf("res: %v\n", string(out))
	d := AgentResponse(out)
	fmt.Printf("agent resp: %v\n", d)

	// write all gh commands to temp bash script and exec it
	// this avoids strings.Split(d,"\n") which would split inside body="...\n..." as well
	// shell will expand ${CI_REPO} etc, so no Go ExpandEnv
	tmp, err := os.CreateTemp("", "gh-commands-*.sh")
	if err != nil {
		slog.Error("failed to create temp script", "error", err)
		os.Exit(1)
	}
	scriptPath := tmp.Name()
	defer func() {
		tmp.Close()
		_ = os.Remove(scriptPath)
	}()
	if _, err := tmp.WriteString("#!/usr/bin/env bash\nset +e\n"); err != nil {
		slog.Error("failed to write script header", "error", err)
		os.Exit(1)
	}
	// bash treats `code` inside "..." as command substitution -> must escape
	normalized := strings.ReplaceAll(d, "\\`", "`")
	escaped := strings.ReplaceAll(normalized, "`", "\\`")
	if _, err := tmp.WriteString(escaped); err != nil {
		slog.Error("failed to write commands", "error", err)
		os.Exit(1)
	}
	if _, err := tmp.WriteString("\n"); err != nil {
		slog.Error("failed to write trailing newline", "error", err)
		os.Exit(1)
	}
	_ = tmp.Close()
	if err := os.Chmod(scriptPath, 0755); err != nil {
		slog.Error("failed to chmod script", "error", err)
		os.Exit(1)
	}
	fmt.Printf("wrote gh commands to %s\n", scriptPath)
	content, _ := os.ReadFile(scriptPath)
	fmt.Printf("script content:\n%s\n---\n", string(content))
	// ensure CI_REPO is in Env for shell expansion (derive from CI_REPO_URL if missing)
	env := os.Environ()
	if os.Getenv("CI_REPO") == "" && conf.RepoUrl != "" {
		repo := strings.TrimPrefix(conf.RepoUrl, "https://github.com/")
		repo = strings.TrimPrefix(repo, "http://github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		repo = strings.TrimSuffix(repo, "/")
		env = append(env, "CI_REPO="+repo)
		fmt.Printf("derived CI_REPO=%s for shell expansion\n", repo)
	}
	env = append(env, "GH_TOKEN="+conf.GhToken, "GITHUB_TOKEN="+conf.GhToken)
	c := exec.Command("bash", scriptPath)
	c.Env = env
	c.Dir = conf.SourceCodePath
	if err := utils.ExecCmdPiped(c); err != nil {
		slog.Error("gh script failed", "script", scriptPath, "error", err)
		os.Exit(1)
	}

}

func AgentResponse(data []byte) string {
	type Event struct {
		Type string `json:"type"`
		Part struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"part"`
	}
	var b strings.Builder
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Type == "text" && e.Part.Text != "" {
			b.WriteString(e.Part.Text)
		}
	}
	raw := strings.TrimSpace(b.String())
	if raw == "" {
		return ""
	}
	extract := func(s string) string {
		// try strict JSON first
		var direct struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(s), &direct); err == nil && direct.Message != "" {
			return direct.Message
		}
		// fix common invalid escape: \` -> ` (model puts \` inside JSON string)
		fixed := strings.ReplaceAll(s, "\\`", "`")
		// also fix \' if present
		fixed = strings.ReplaceAll(fixed, "\\'", "'")
		if err := json.Unmarshal([]byte(fixed), &direct); err == nil && direct.Message != "" {
			return direct.Message
		}
		// manual fallback: find "message":" and scan for closing unescaped quote
		idx := strings.Index(s, "\"message\"")
		if idx == -1 {
			idx = strings.Index(s, "'message'")
		}
		if idx == -1 {
			return ""
		}
		colon := strings.Index(s[idx:], ":")
		if colon == -1 {
			return ""
		}
		start := idx + colon + 1
		// skip spaces and find opening quote
		for start < len(s) && (s[start] == ' ' || s[start] == '\n' || s[start] == '\r' || s[start] == '\t') {
			start++
		}
		if start >= len(s) || s[start] != '"' {
			return ""
		}
		start++ // after opening "
		var out strings.Builder
		escaped := false
		for i := start; i < len(s); i++ {
			c := s[i]
			if escaped {
				// handle \" , \\ , \` already fixed, but keep general
				if c == '"' || c == '\\' || c == '`' || c == '\'' || c == 'n' || c == 't' {
					if c == 'n' {
						out.WriteByte('\n')
					} else if c == 't' {
						out.WriteByte('\t')
					} else {
						out.WriteByte(c)
					}
				} else {
					out.WriteByte(c)
				}
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				return out.String()
			}
			out.WriteByte(c)
		}
		return ""
	}

	// 1. direct
	if msg := extract(raw); msg != "" {
		return msg
	}
	// 2. ```json block
	if start := strings.Index(raw, "```json"); start != -1 {
		start += len("```json")
		if end := strings.Index(raw[start:], "```"); end != -1 {
			block := strings.TrimSpace(raw[start : start+end])
			if msg := extract(block); msg != "" {
				return msg
			}
			return block
		}
	}
	if start := strings.Index(raw, "```"); start != -1 {
		start += len("```")
		if end := strings.Index(raw[start:], "```"); end != -1 {
			block := strings.TrimSpace(raw[start : start+end])
			if msg := extract(block); msg != "" {
				return msg
			}
			return block
		}
	}
	// 3. first { to last }
	if first := strings.Index(raw, "{"); first != -1 {
		if last := strings.LastIndex(raw, "}"); last != -1 && last > first {
			candidate := raw[first : last+1]
			if msg := extract(candidate); msg != "" {
				return msg
			}
			return candidate
		}
	}
	return raw
}

type OpenCodeError struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	SessionID string `json:"sessionID"`
	Error     struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
			Ref     string `json:"ref"`
		} `json:"data"`
	} `json:"error"`
}

func parseOpenCodeError(data []byte) *OpenCodeError {
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var e OpenCodeError
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Type == "error" && e.Error.Name != "" {
			return &e
		}
	}
	return nil
}

func safeEnv(apiKey string) []string {
	allow := map[string]bool{
		"HOME":   true,
		"USER":   true,
		"SHELL":  true,
		"LANG":   true,
		"LC_ALL": true,
		"TERM":   true,
	}
	var env []string
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		if allow[k] {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME=/root")
	env = append(env, "TMPDIR=/tmp")
	env = append(env, "TMP=/tmp")
	env = append(env, "OPENCODE_API_KEY="+apiKey)
	return env
}
