# op-reviewer

`op-reviewer` is a tool that uses [OpenCode](https://opencode.ai/) to review the
latest pull-request commit and publish the result to GitHub. Executes in docker container, its not depending on any specific CI runner.

> currently its meant for woodpecker CI

## How It Works

1. Reads CI metadata and selects an OpenCode API key for the commit author.
2. Clones `CI_REPO_URL` into `./source` when a checkout is not already present.
3. Checks out `CI_COMMIT_SHA`.
4. Runs OpenCode in the checked-out repository and asks it to review the diff.
5. Parses OpenCode's JSON response for generated `gh api` commands.
6. Executes those commands to create a pull-request summary and optional inline comments.

The container includes the compiled reviewer binary, OpenCode, GitHub CLI, and Git.
The repository being reviewed is fetched at runtime and is not included in the image.

## Configuration

Environment variables:

| Variable | Purpose |
| --- | --- |
| `CI_REPO_URL` | Repository URL used when cloning the review target. |
| `CI_COMMIT_SHA` | Commit to check out and review. |
| `CI_COMMIT_PULL_REQUEST` | Pull-request number used by GitHub API calls. |
| `CI_REPO` | GitHub repository in `owner/name` form. Optional when `CI_REPO_URL` is a GitHub URL. |
| `CI_COMMIT_AUTHOR` | Commit author's GitHub username, used to select an API key. |
| `OP_REVIEWER_MODEL` | OpenCode model identifier. |
| `OP_REVIEWER_GH_TOKEN` | GitHub token used to publish the review. |

Author-specific OpenCode keys use the form `OP_REVIEWER_<author>`. Underscores in
the suffix are treated as hyphens when matching GitHub usernames. For example:

```text
CI_COMMIT_AUTHOR=antoni-ostrowski
OP_REVIEWER_antoni_ostrowski=<opencode-api-key>
```

## Run With Docker

Build the image:

```sh
docker build -t op-reviewer .
```

Run it with CI variables and secrets:

```sh
docker run --rm --env-file .env.local op-reviewer
```

The image expects to start in `/app`; it creates the runtime checkout at
`/app/source`.

## Development

Install the Go version declared in `mise.toml`, then run:

```sh
mise run dev
```

Build the binary locally:

```sh
mise run build
```
