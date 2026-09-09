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

	prompt := `
UNDER ANY CIRCUMSTANCES DO NOT READ OS ENVS DIRECTLY via env/printenv/cat /proc/self/environ or by reading files that contain secrets. Use only placeholders.

You are a code review agent. You are running inside the repo (cwd is repo root) already checked out at ${CI_COMMIT_SHA}. Do not clone.

1. Find changes: run git diff HEAD~1, git show --stat, git log -1 --name-only to get changed files and diff. Review only the diff, but use the repo to gain context about the change if needed/unclear.

2. Before reviewing, inspect existing pull-request reviews and inline comments with these read-only commands:
- gh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/reviews
- gh api repos/${CI_REPO}/pulls/${CI_COMMIT_PULL_REQUEST}/comments
Use them to identify findings already reported anywhere on this PR. Do not repeat an existing finding, even if it was reported on an earlier commit. Report only genuinely new findings introduced by the latest diff.

3. Review for: bugs, logic errors, security issues, error handling, code quality, performance.

4. Return ONLY one JSON object matching {"body":"summary","comments":[{"body":"finding","path":"file","line":12}]}. No markdown, no explanation, no extra keys. If there are no new findings, return {"body":"","comments":[]}.
Do not return shell commands, scripts, tool calls, or progress prose. Do not write files or temporary scripts. Do not ask for confirmation. CI runs without a user present, so use only non-interactive, read-only commands while gathering context. The application adds the review attribution; do not add it yourself.

5. Use ONLY these bash placeholders with ${VAR} syntax:
- ${CI_COMMIT_SHA} - commit SHA to review
- ${CI_REPO} - owner/repo (e.g. octocat/hello-world) — use this for gh api paths
- ${CI_COMMIT_PULL_REQUEST} - pull request number
- gh CLI is already authenticated via GH_TOKEN. Read-only gh api commands above are allowed for gathering context. Do not execute write commands. Do not handle auth.

6. For inline comments, line MUST be the absolute line number in the current file, not a diff position, offset, or file index. Use only lines present on the new/current side of the diff. Read the hunk header from git diff HEAD~1 or the GitHub patch, then map the selected added line to its current file line number. Never use position.`

	cmd := exec.Command("opencode", "run", "--auto", "--format", "json", "--model", conf.Model, "--log-level", "DEBUG", "--print-logs", strings.TrimSpace(prompt))
	cmd.Env = safeEnv(conf.AiApiKey, conf.GhToken)
	cmd.Dir = conf.SourceCodePath
	out, err := cmd.CombinedOutput()
	if err != nil {
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

	d := AgentResponse(out)
	review, err := parseReviewResponse(d)
	if err != nil {
		slog.Error("invalid agent review response", "error", err, "response", d)
		os.Exit(1)
	}
	if err := publishReview(conf, review); err != nil {
		slog.Error("failed to publish review", "error", err)
		os.Exit(1)
	}

}

type ReviewResponse struct {
	Body     string          `json:"body"`
	Comments []ReviewComment `json:"comments"`
}

type ReviewComment struct {
	Body string `json:"body"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

func parseReviewResponse(data string) (ReviewResponse, error) {
	var review ReviewResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &review); err != nil {
		return ReviewResponse{}, fmt.Errorf("expected JSON review object: %w", err)
	}
	for i, comment := range review.Comments {
		if strings.TrimSpace(comment.Body) == "" {
			return ReviewResponse{}, fmt.Errorf("comment %d has empty body", i)
		}
		if strings.TrimSpace(comment.Path) == "" {
			return ReviewResponse{}, fmt.Errorf("comment %d has empty path", i)
		}
		if comment.Line < 1 {
			return ReviewResponse{}, fmt.Errorf("comment %d has invalid line %d", i, comment.Line)
		}
	}
	return review, nil
}

func publishReview(conf *config.Config, review ReviewResponse) error {
	if strings.TrimSpace(review.Body) == "" && len(review.Comments) == 0 {
		slog.Info("review completed with no new findings")
		return nil
	}

	repo := os.Getenv("CI_REPO")
	if repo == "" {
		repo = strings.TrimPrefix(conf.RepoUrl, "https://github.com/")
		repo = strings.TrimPrefix(repo, "http://github.com/")
		repo = strings.TrimSuffix(strings.TrimSuffix(repo, ".git"), "/")
	}
	pullRequest := os.Getenv("CI_COMMIT_PULL_REQUEST")
	if repo == "" || pullRequest == "" {
		return fmt.Errorf("CI_REPO and CI_COMMIT_PULL_REQUEST are required to publish review")
	}

	env := os.Environ()
	env = append(env, "GH_TOKEN="+conf.GhToken, "GITHUB_TOKEN="+conf.GhToken)

	for i, comment := range review.Comments {
		args := []string{
			"api", fmt.Sprintf("repos/%s/pulls/%s/comments", repo, pullRequest),
			"-f", "body=" + comment.Body,
			"-f", "commit_id=" + conf.SHA,
			"-f", "path=" + comment.Path,
			"-F", fmt.Sprintf("line=%d", comment.Line),
			"-f", "side=RIGHT",
		}
		if err := runGH(conf.SourceCodePath, env, args...); err != nil {
			return fmt.Errorf("inline comment %d: %w", i, err)
		}
	}
	if review.Body != "" {
		review.Body += "\n\nAI review by op-reviewer"
		args := []string{
			"api", fmt.Sprintf("repos/%s/pulls/%s/reviews", repo, pullRequest),
			"-f", "event=COMMENT",
			"-f", "body=" + review.Body,
			"-f", "commit_id=" + conf.SHA,
		}
		if err := runGH(conf.SourceCodePath, env, args...); err != nil {
			return fmt.Errorf("summary review: %w", err)
		}
	}

	return nil
}

func runGH(dir string, env []string, args ...string) error {
	slog.Info("publishing review item")
	cmd := exec.Command("gh", args...)
	cmd.Env = env
	cmd.Dir = dir
	return utils.ExecCmdPiped(cmd)
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

func safeEnv(apiKey string, ghToken string) []string {
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
	env = append(env, "GH_TOKEN="+ghToken)
	for _, key := range []string{"CI_REPO", "CI_COMMIT_SHA", "CI_COMMIT_PULL_REQUEST"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}
