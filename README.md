# gitee-cli

**English** | [简体中文](README.zh-CN.md)

A command-line tool for interacting with [Gitee](https://gitee.com), built on the [Gitee OpenAPI v5](https://gitee.com/api/v5/swagger).

Manage authentication, pull requests, issues, repositories, and CI status from your terminal — with an output style consistent with `gh` / `glab`.

## Status

- ✅ Phase 1 — Project scaffolding (done)
- ✅ Phase 1 — Authentication module (done)
- ✅ PR create / list / view / checkout (done)
- ✅ Issue list / view (done)
- ✅ CI status (done)

The current release ships the project skeleton, configuration management, an API client wrapper, full authentication, and PR / Issue / Repo / CI capabilities.

## Features

The first release covers the following commands:

| Command | Description |
| --- | --- |
| `gitee auth login` | Interactive or token-based login |
| `gitee auth status` | Show the current login state |
| `gitee auth logout` | Log out and clear local credentials |
| `gitee repo view` | View repository information |
| `gitee repo clone` | Clone a repository with authentication |
| `gitee pr create` | Create a Pull Request (interactive or flags) |
| `gitee pr list` | List PRs with state / author / label / branch filters |
| `gitee pr view` | View Pull Request details |
| `gitee pr checkout` | Check out a PR into a local branch |
| `gitee pr review` | Review a PR (`--approve` to approve / `--comment` to comment) |
| `gitee issue list` | List issues with filters |
| `gitee issue view` | View issue details |
| `gitee ci status` | View CI/CD build status for the repository |

Output matches the conventions of `gh` / `glab`, and most read commands support `--json` for scripting.

## Requirements

- Go 1.25+ (only required when building from source or using `go install`)
- Git (used for repository detection and branch operations)

## Quick Start

```bash
# 1. Install (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash

# 2. Log in (generate a token at https://gitee.com/profile/personal_access_tokens)
gitee auth login

# 3. Verify
gitee auth status

# 4. Create a PR from the current branch
gitee pr create --title "Add new feature" --body "This PR adds ..."
```

## Installation

### One-line install script (recommended)

**macOS / Linux / Git Bash / WSL:**

```bash
curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash
```

**Windows PowerShell:**

```powershell
irm https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.ps1 | iex
```

The PowerShell script auto-detects the architecture (x64 / arm64), downloads the matching `gitee.exe` from GitHub Releases, installs it to `%LOCALAPPDATA%\Programs\gitee`, and adds it to the user `PATH`. Reopen your terminal afterwards to use `gitee`.

> If you hit an execution-policy restriction, relax it for the current session only:
> `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`

### Manual download

1. Download the archive for your platform from the [latest release](https://github.com/Pipreola/gitee-cli/releases/latest):
   - Linux / macOS: `gitee_<version>_<OS>_<arch>.tar.gz`
   - Windows: `gitee_<version>_Windows_<arch>.zip`
2. Extract the archive to obtain the `gitee` (or `gitee.exe`) binary.
3. Move it to a directory on your `PATH`:
   - macOS / Linux: `sudo mv gitee /usr/local/bin/`
   - Windows: place `gitee.exe` in a folder such as `%LOCALAPPDATA%\Programs\gitee` and add that folder to your user `PATH`.
4. Verify: `gitee version`

### Using `go install`

```bash
go install github.com/Pipreola/gitee-cli@latest
```

### Build from source

```bash
git clone https://github.com/Pipreola/gitee-cli.git
cd gitee-cli
go build -o gitee .
```

## Project Layout

```
.
├── main.go              # Program entry point
├── cmd/                 # Cobra command tree (root / version / auth / pr / issue / repo / ci)
├── pkg/
│   ├── api/             # Gitee OpenAPI v5 client (auth, PR, issue, repo, CI, error handling)
│   ├── config/          # Config read/write (~/.config/gitee-cli/config.yaml)
│   └── auth/            # Authentication logic bridging config and API
├── internal/
│   └── version/         # Version metadata
└── docs/api/            # OpenAPI specs
```

## Usage

### Authentication

Generate a personal access token at https://gitee.com/profile/personal_access_tokens. Recommended scopes: `user_info`, `projects`, `pull_requests`, `issues`.

```bash
# Interactive login (recommended; token input is hidden)
gitee auth login

# Login with a flag
gitee auth login --token <your-personal-access-token>

# Show current login state
gitee auth status

# Log out and clear local credentials
gitee auth logout

# Show version
gitee version
```

### Pull Requests

```bash
# Interactive create (recommended)
gitee pr create

# Create with flags
gitee pr create --title "Add new feature" --body "This PR adds ..."

# Draft PR
gitee pr create --draft --title "WIP: Refactor auth module"

# Reviewers and labels
gitee pr create --title "Fix bug" --assignees user1,user2 --labels bug,urgent

# Open the PR in a browser after creating
gitee pr create --title "Update docs" --web

# List open PRs in the current repo (default --state open)
gitee pr list

# List merged PRs / PRs you authored
gitee pr list --state merged
gitee pr list --author @me

# JSON output for scripting
gitee pr list --json --limit 50

# View PR details (with comments)
gitee pr view 123 --comments

# Check out a PR into a local branch named pr-<number>
gitee pr checkout 123
gitee pr checkout https://gitee.com/owner/repo/pulls/456

# Review a PR
gitee pr review 123 --approve                                          # approve
gitee pr review 123 --approve --body "LGTM, looks clean"               # approve with a comment
gitee pr review 123 --approve --force                                  # force-approve (bypass branch protection rules)
gitee pr review 123 --comment --body "Line 10: please add bounds check" # comment-only review
```

### Issues

```bash
# List issues (filter by assignee, state, labels, ...)
gitee issue list
gitee issue list --assignee @me

# View issue details with comments
gitee issue view IABCDE --comments
```

### Repository & CI

```bash
# View repository info (current repo or owner/repo)
gitee repo view
gitee repo view owner/repo --json

# Clone a repository using the saved token (origin is reset to a token-free URL afterwards)
gitee repo clone owner/repo
gitee repo clone owner/repo my-dir
# Pass extra args through to git after --
gitee repo clone owner/repo -- --depth 1 --branch dev

# View CI/CD status for the current branch
gitee ci status
gitee ci status --ref abc1234 --json
```

Run `gitee <command> --help` for the full flag list of any command.

## Configuration

Default path: `~/.config/gitee-cli/config.yaml`. The token is stored with **0600** permissions.

```yaml
host: https://gitee.com/api/v5
token: <your-token>
user: <login>
```

The config directory can be overridden with the `GITEE_CLI_CONFIG_DIR` environment variable, which is handy for testing and custom deployments.

## Testing

```bash
go test ./... -cover
```

## API Docs

OpenAPI 3.0 specs live in `docs/api/`: `user`, `pr`, `issue`, `repo`, and `ci`.

## Claude / Multica Skill

The repository ships a skill guide for Claude / Multica agents covering all first-release commands with parameters, runnable examples, auth prerequisites, and common error handling:

- `.claude/skills/gitee-cli/SKILL.md`

## Contributing

Issues and pull requests are welcome. Please run `go test ./...` and `gofmt` before submitting, and reference the related issue in your commits. See [CHANGELOG.md](CHANGELOG.md) for the release history.

## License

Licensed under the [Apache License 2.0](LICENSE).
