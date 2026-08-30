package main

import (
	"log/slog"
	"os"
	"os/exec"

	"github.com/antoni-ostrowski/op-reviewer/internal/config"
	"github.com/antoni-ostrowski/op-reviewer/internal/utils"
)

// OP_REVIEWER_<git-author-nickname>
// OP_REVIEWER_GH_TOKEN
// OP_REVIEWER_MODEL

func main() {
	conf, err := config.New()
	if err != nil {
		slog.Error("failed to configure app", "error", err)
		os.Exit(1)
	}

	slog.Info("sucessfully created config", "details", conf)

	utils.ExecCmdPiped(exec.Command("ls", "-al"))
	utils.ExecCmdPiped(exec.Command("ls", "-al", conf.SourceCodePath))
	// cmd := exec.Command("env")
	// cmd.Env = []string{}
	// execCmdPiped(cmd)
	// pipeExecCmd(exec.Command("ls", "-al"))
	// repoUrl := os.Getenv("CI_REPO_URL") // or GITHUB_REPOSITORY
	// sha := os.Getenv("CI_COMMIT_SHA")
	// if _, err := os.Stat(".git"); os.IsNotExist(err) {
	// 	pipeExecCmd(exec.Command("git", "clone", repoUrl, "./repo"))
	// 	pipeExecCmd(exec.Command("git", "checkout", sha))
	// }
	// slog.Info("cloned repo: ", "url", repoUrl, "sha", sha)
	// pipeExecCmd(exec.Command("ls", "-al"))
	//
	// envs := os.Environ()
	// apiTokens := getApiTokens(envs)
	// apiToken := selectApiToken(apiTokens)
	// _ = apiToken
	//
	// model := os.Getenv("OP_REVIEWER_MODEL")
	// _ = model
	// slog.Info("hello from op reviewer")

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
