package config

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/antoni-ostrowski/op-reviewer/internal/utils"
)

const ENV_PREFIX = "OP_REVIEWER_"

type Config struct {
	Model          string
	AiApiKey       string
	RepoUrl        string
	SHA            string
	GhToken        string
	SourceCodePath string
}

func New() (*Config, error) {
	conf := &Config{}
	env := os.Environ()
	commitAuthor := os.Getenv("CI_COMMIT_AUTHOR")

	conf.AiApiKey = conf.findMatchingApiKey(env, commitAuthor)
	conf.Model = os.Getenv(ENV_PREFIX + "MODEL")
	conf.RepoUrl = os.Getenv("CI_REPO_URL")
	conf.SHA = os.Getenv("CI_COMMIT_SHA")
	conf.GhToken = os.Getenv("OP_REVIEWER_GH_TOKEN")
	conf.SourceCodePath = "./source"

	if _, err := os.Stat(filepath.Join(conf.SourceCodePath, ".git")); os.IsNotExist(err) {
		slog.Info("git: repo not found in source code path, fetching via repo url", "repo_url", conf.RepoUrl)
		if err := utils.ExecCmdPiped(exec.Command("git", "clone", conf.RepoUrl, conf.SourceCodePath)); err != nil {
			return nil, fmt.Errorf("config: failed to clone repo: %v", err)
		}
	}

	slog.Info("git: checking out", "sha", conf.SHA)
	if err := utils.ExecCmdPiped(exec.Command("git", "-C", conf.SourceCodePath, "checkout", conf.SHA)); err != nil {
		return nil, fmt.Errorf("config: failed to clone repo: %v", err)
	}

	return conf, nil
}

func (c *Config) findMatchingApiKey(envs []string, commitAuthor string) string {
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

	for key, value := range validKeys {
		if strings.Contains(key, commitAuthor) {
			return value
		}
	}

	return ""
}

// replace "_" to "-"
//
// gh usernames dont have "_" but allow "-" which are not valid env keys
func decodeEnvKey(s string) string {
	suffix := strings.TrimPrefix(s, ENV_PREFIX)
	return ENV_PREFIX + strings.ReplaceAll(suffix, "_", "-")
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("model", c.Model),
		slog.String("repo_url", c.RepoUrl),
		slog.String("commit_sha", c.SHA),
	)
}
