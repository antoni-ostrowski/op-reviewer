package main

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// OP_REVIEWER_<git-author-nickname>
// OP_REVIEWER_GH_TOKEN
// OP_REVIEWER_MODEL
const ENV_PREFIX = "OP_REVIEWER_"

func main() {

	pipeExecCmd(exec.Command("ls", "-al"))
	repoUrl := os.Getenv("CI_REPO_URL") // or GITHUB_REPOSITORY
	sha := os.Getenv("CI_COMMIT_SHA")
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		pipeExecCmd(exec.Command("git", "clone", repoUrl, "./repo"))
		pipeExecCmd(exec.Command("git", "checkout", sha))
	}
	slog.Info("cloned repo: ", "url", repoUrl, "sha", sha)
	pipeExecCmd(exec.Command("ls", "-al"))

	envs := os.Environ()
	apiTokens := getApiTokens(envs)
	apiToken := selectApiToken(apiTokens)
	_ = apiToken

	model := os.Getenv("OP_REVIEWER_MODEL")
	_ = model
	slog.Info("hello from op reviewer")

	// prompt := `
	// DONT EVER TRY TO READ ANY ENV FILES OR LOG THEM
	// You are a code reviewer, you have access to these envs:
	// - OP_REVIEWER_GH_TOKEN
	// - CI_COMMIT_SHA
	// - CI_COMMIT_PULL_REQUEST
	// - CI_REPO
	// You are running in env that has full access to repo, compare diff on latest commit using env and review the changes, you can use repo to gain context.
	// Run gh cli commands to publish comments and reviews on PR, using the $ENV syntax
	// `
	// cmd := exec.Command("opencode", "run", strings.TrimSpace(prompt))
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// cmd.Run()

}

func getApiTokens(envs []string) map[string]string {
	validKeys := make(map[string]string)

	for _, v := range envs {
		envStrArr := strings.SplitN(v, "=", 2)
		if len(envStrArr) != 2 {
			continue
		}

		envKey := envStrArr[0]
		envValue := envStrArr[1]

		if strings.HasPrefix(envKey, ENV_PREFIX) {
			decoded := decodeEnvKey(envKey)
			validKeys[decoded] = envValue
		}
	}

	for key, value := range validKeys {
		slog.Info("valid op api key", "name", key, "value", value)
	}
	return validKeys
}

// replace "_" to "-"
//
// gh usernames dont have "_" but allow "-" which are not valid env keys
func decodeEnvKey(s string) string {
	suffix := strings.TrimPrefix(s, ENV_PREFIX)
	return ENV_PREFIX + strings.ReplaceAll(suffix, "_", "-")
}

func selectApiToken(envs map[string]string) string {
	commitAuthor := os.Getenv("CI_COMMIT_AUTHOR")
	for key, value := range envs {
		if strings.Contains(key, commitAuthor) {
			return value
		}
	}
	return ""
}
func pipeExecCmd(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
