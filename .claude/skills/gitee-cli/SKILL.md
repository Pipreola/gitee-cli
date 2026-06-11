---
name: gitee-cli
description: Usage manual for the phase-1 gitee CLI commands. Covers the parameters, runnable examples, authentication prerequisites, and common error handling for all 9 commands — auth login/status, repo view, pr list/view/create/checkout, issue list/view, and ci status. Invoke this skill when the user needs to inspect repository info, list / create / check out PRs, query issues, or view CI status under a Gitee repository.
---

# gitee CLI Manual (Phase 1)

`gitee` is a command-line tool modeled after the `gh` / `glab` style, built on the [Gitee OpenAPI v5](https://gitee.com/api/v5/swagger). This document **reflects the merged code as the source of truth** and covers all 9 phase-1 commands.

> The CLI emits its own user-facing messages (errors, success notices) in Chinese. Those literal strings are quoted verbatim below so that what you see in the terminal matches this manual exactly; English glosses are provided alongside.

## Table of Contents

- [Authentication Prerequisites (read first)](#authentication-prerequisites-read-first)
- [auth login](#auth-login) / [auth status](#auth-status) / [auth logout](#auth-logout-supplementary)
- [repo view](#repo-view)
- [pr list](#pr-list) / [pr view](#pr-view) / [pr create](#pr-create) / [pr checkout](#pr-checkout)
- [issue list](#issue-list) / [issue view](#issue-view)
- [ci status](#ci-status)
- [Config File and Environment Variables](#config-file-and-environment-variables)
- [Common Errors and Troubleshooting](#common-errors-and-troubleshooting)

---

## Authentication Prerequisites (read first)

Except for `gitee auth login` / `gitee version`, **all commands require a local login** (a valid token saved in the config). When not logged in, commands fail immediately:

```
未登录，请先运行 'gitee auth login' 进行认证
```

(*"Not logged in, please run 'gitee auth login' to authenticate first."*)

### Where the token comes from

Visit https://gitee.com/profile/personal_access_tokens to create a personal access token. The following scopes are recommended:

- `user_info` — needed by `auth status`, `pr list --author @me`, etc. to identify the current user
- `projects` — needed by `repo view`
- `pull_requests` — needed by `pr list/view/create/checkout`
- `issues` — needed by `issue list/view`

### Where the token is stored

After `gitee auth login` validates the token, it writes to **`~/.config/gitee-cli/config.yaml`** (permission `0600`). The file looks like:

```yaml
host: https://gitee.com/api/v5
token: <your-token>
user: <your-login>
```

You can override the config directory with the environment variable `GITEE_CLI_CONFIG_DIR` (useful for testing or switching between multiple accounts).

### Repository resolution

Except for `repo view [owner/repo]`, which accepts an explicit argument, all commands infer `owner/repo` from the **git remote `origin` of the current directory**. Two remote URL formats are supported:

- `https://gitee.com/owner/repo.git`
- `git@gitee.com:owner/repo.git`

When not inside a git repository, or when `origin` points to a non-Gitee domain, the `pr` / `issue` / `ci` commands will fail.

---

## auth login

Log in to Gitee with a personal access token; on successful validation, the credential is written to the local config.

### Parameters

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--token` | string | empty | Gitee personal access token; if empty, enters interactive no-echo input |
| `--host` | string | `https://gitee.com/api/v5` | Gitee API base address (used for self-hosted deployments) |

### Examples

```bash
# Recommended: interactive input; the token is not echoed to the terminal
gitee auth login

# Pass directly via flag (CI scenarios)
gitee auth login --token <your-personal-access-token>

# Self-hosted deployment
gitee auth login --token <token> --host https://gitee.example.com/api/v5
```

Success output:

```
✓ 登录成功，当前用户: 张三 (zhangsan)
```

(*"✓ Login succeeded, current user: ..."*)

### Common Errors

- **`读取密码失败（请确认在终端中运行或使用 --token 参数）`** (*"Failed to read password (make sure you run in a terminal or use the --token flag)"*): running `gitee auth login` in a non-TTY environment (pipe, CI, IDE integrated terminal) without `--token`. Use `gitee auth login --token <token>` instead.
- **`令牌不能为空`** (*"Token cannot be empty"*): pressing Enter directly during interactive input, or the pasted content was interpreted as empty.
- **`令牌校验失败: ...`** (*"Token validation failed: ..."*): the token has expired, been revoked, or lacks sufficient scope. Generate a new token at https://gitee.com/profile/personal_access_tokens.

## auth status

Show the current login status.

### Parameters

None.

### Examples

```bash
gitee auth status
```

Logged-in output:

```
✓ 已登录
用户: zhangsan
主页: https://gitee.com/zhangsan
```

(*"✓ Logged in / User: ... / Homepage: ..."*)

Not-logged-in output:

```
✗ 未登录
使用 'gitee auth login' 命令登录
```

(*"✗ Not logged in / Use the 'gitee auth login' command to log in"*)

### Common Errors

- **`令牌校验失败（可能已过期）: ...`** (*"Token validation failed (possibly expired): ..."*): the locally saved token still exists but has been revoked server-side. Run `gitee auth logout`, then `gitee auth login` again.

## auth logout (supplementary)

> Not in the phase-1 list of 9 commands, but belongs to the same `auth` command group as `auth login/status`; it is implemented and usable.

Clear the local credential.

```bash
gitee auth logout
# ✓ 已登出，本地凭证已清除   ("✓ Logged out, local credential cleared")
```

---

## repo view

View a repository's basic information and statistics.

### Usage

```
gitee repo view [owner/repo] [flags]
```

Without the positional argument, the repository is inferred from the `origin` remote of the current directory.

### Parameters

| Flag / positional | Type | Default | Description |
| --- | --- | --- | --- |
| `[owner/repo]` | string | current git remote | Explicitly target any repository, e.g. `oschina/git-osc` |
| `--web` / `-w` | bool | false | Skip printing details and open the repository homepage in the browser |
| `--json` | bool | false | Output the raw OpenAPI JSON |
| `--no-color` | bool | false | Disable ANSI color (auto-disabled when stdout is not a TTY, e.g. piped) |

### Examples

```bash
# View the current repository
gitee repo view

# View a specific repository
gitee repo view oschina/git-osc

# Output JSON for jq processing
gitee repo view --json | jq '.stargazers_count'

# Open in the browser
gitee repo view -w
```

### Common Errors

- **`无效的仓库定位 "...", 期望格式 owner/repo`** (*"Invalid repository locator '...', expected format owner/repo"*): the positional argument is missing the `/` or has an empty segment.
- **`获取仓库信息失败: ... 404`** (*"Failed to fetch repository info: ... 404"*): the repository does not exist, is inaccessible, or has been deleted.

---

## pr list

List the Pull Requests of the current repository; the output format aligns with `gh pr list` / `glab mr list`.

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--state` | `-s` | string | `open` | `open` / `closed` / `merged` / `all` |
| `--author` | `-A` | string | empty | Filter by author username; `@me` means the current logged-in user (client-side filter) |
| `--label` | `-l` | string | empty | Filter by labels, comma-separated, e.g. `bug,urgent` |
| `--base` | | string | empty | Filter by target branch |
| `--head` | | string | empty | Filter by source branch, format `namespace:branch` |
| `--sort` | | string | `created` | `created` / `updated` / `popularity` / `long-running` |
| `--direction` | | string | `desc` | `asc` / `desc` |
| `--limit` | `-L` | int | `30` | 1–100 |
| `--json` | | bool | false | Output the raw JSON array |
| `--verbose` | `-v` | bool | false | Multi-column display with author and update time |
| `--no-color` | | bool | false | Disable color |

### Examples

```bash
# Default: all open PRs of the current repository
gitee pr list

# Merged + descending by update time
gitee pr list --state merged --sort updated --direction desc

# My PRs (@me available after login)
gitee pr list --author @me

# Multi-label filter + verbose mode
gitee pr list --label bug,urgent --verbose

# Output JSON for script processing
gitee pr list --state all --limit 100 --json | jq '.[].number'

# Filter by target branch
gitee pr list --base main
```

### Common Errors

- **`无效的 --state 值 "draft"，仅支持 open/closed/merged/all`** (*"Invalid --state value 'draft', only open/closed/merged/all are supported"*): Gitee v5 does not distinguish a draft state; use `open` and filter within the results.
- **`无效的 --limit 值 200，须在 1-100 之间`** (*"Invalid --limit value 200, must be between 1-100"*): the per-call ceiling is 100; paginate via scripting if you need more.
- **`无法解析 @me：本地配置未保存登录用户名，请重新执行 'gitee auth login'`** (*"Cannot resolve @me: the local config has no saved login username, please run 'gitee auth login' again"*): a config from an early login lacks the `user` field; log in again to fix it.

---

## pr view

View the details of a specific PR.

### Usage

```
gitee pr view <number> [flags]
```

`<number>` must be a positive integer.

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--web` | `-w` | bool | false | Open the PR page (it still fetches PR details first to obtain the URL; it does not fetch comments or print details) |
| `--comments` | `-c` | bool | false | Additionally show the PR comment list |
| `--json` | | bool | false | Output JSON (`{pull_request, comments?}` structure) |
| `--no-color` | | bool | false | Disable color |

### Examples

```bash
# View PR #123
gitee pr view 123

# Show comments as well
gitee pr view 123 --comments

# JSON output (with comments)
gitee pr view 123 --comments --json

# Open in the browser directly
gitee pr view 123 -w
```

### Common Errors

- **`无效的 PR 编号 "abc"，必须是正整数`** (*"Invalid PR number 'abc', must be a positive integer"*): the argument must be a number; URL form is supported only in `pr checkout`.
- **`获取 PR 详情失败: ... 404`** (*"Failed to fetch PR details: ... 404"*): the PR number does not exist or the repository is inaccessible.

---

## pr create

Create a new Pull Request. If no title/body is given, it enters interactive prompts.

### Behavior Notes (from the code)

- `--head` defaults to the **current branch**; `--base` defaults to `origin/main`, falling back to `origin/master` if it does not exist, and erroring to require an explicit `--base` if neither exists.
- Before creating, it checks whether the `--head` branch has been pushed to origin; if not, it **automatically runs `git push -u origin <branch>`**.
- `head == base` errors immediately.
- If `--body` is not given and the `--body` flag was never explicitly set (even as `""`), it enters interactive body input; an explicit `--body ""` is honored as an empty body without prompting.

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--title` | `-t` | string | interactive | PR title; cannot be empty during interactive input |
| `--body` | `-b` | string | interactive | PR body, optional |
| `--base` | | string | auto | Target branch, defaults to `main` or `master` |
| `--head` | | string | current branch | Source branch |
| `--draft` | `-d` | bool | false | Create as a draft PR |
| `--labels` | `-l` | string | empty | Labels, comma-separated |
| `--milestone` | `-m` | int64 | 0 | Milestone number; 0 means unset |
| `--assignees` | `-a` | string | empty | Reviewer usernames, comma-separated |
| `--testers` | | string | empty | Tester usernames, comma-separated |
| `--web` | `-w` | bool | false | Open in the browser after creation |

### Examples

```bash
# Fully interactive creation
gitee pr create

# One-liner creation
gitee pr create --title "Add new feature" --body "This PR adds ..."

# Draft PR
gitee pr create --draft --title "WIP: Refactor auth module"

# Specify reviewers + labels
gitee pr create --title "Fix bug" \
  --assignees user1,user2 \
  --labels bug,urgent

# Custom source/target branches
gitee pr create --title "Cherry pick to release" \
  --head feature/x --base release/1.0

# Associate a milestone and auto-open the browser
gitee pr create -t "Update docs" -m 42 -w
```

### Common Errors

- **`源分支和目标分支不能相同（当前都是 main）`** (*"Source and target branches cannot be the same (both are currently main)"*): the current branch is `main`; switch to a feature branch first.
- **`找不到默认分支（main 或 master），请使用 --base 指定目标分支`** (*"Default branch not found (main or master), please use --base to specify the target branch"*): the remote has neither `origin/main` nor `origin/master`; specify something like `--base release/1.0` explicitly.
- **`分支 'feature/x' 尚未推送到远程，正在推送...`** (*"Branch 'feature/x' has not been pushed to the remote, pushing..."*) followed by **`推送分支失败`** (*"Failed to push branch"*): usually a remote permission or network issue; verify manually first with `git push -u origin feature/x`.
- **`PR 标题不能为空`** (*"PR title cannot be empty"*): pressing Enter directly during interactive input.
- **`创建 PR 失败: ... 422`** (*"Failed to create PR: ... 422"*): common causes — an open PR with the same source/target branches already exists, the target repository does not have PRs enabled, or the source branch has no new commits.

---

## pr checkout

Check out a PR to a local branch; the local branch is uniformly named `pr-<number>`.

### Usage

```
gitee pr checkout <number|url> [flags]
```

Two input forms are supported:

- A plain numeric PR number, e.g. `123`
- A Gitee PR URL, e.g. `https://gitee.com/owner/repo/pulls/456`

> **Checking out by branch name is not yet supported** (the code comment states explicitly: an additional list-PR API call would be required to resolve the number).

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--force` | `-f` | bool | false | Skip the uncommitted-changes check; when the local branch already exists, hard-reset to the latest PR commit (discarding local differences) |

### Behavior

1. Always runs `git fetch origin pull/<number>/head` into `FETCH_HEAD` first, to avoid the rejection of fetching directly into the currently checked-out branch.
2. Local `pr-<number>` does not exist → `git checkout -b pr-<number> FETCH_HEAD`.
3. Local `pr-<number>` already exists → switch to that branch:
   - by default attempts `git merge --ff-only`, erroring on a non-fast-forward;
   - `--force` switches it to `git reset --hard FETCH_HEAD`.
4. Checking out a closed / merged PR produces a warning but is still allowed.

### Examples

```bash
# Check out by number
gitee pr checkout 123

# Check out by URL
gitee pr checkout https://gitee.com/owner/repo/pulls/456

# pr-123 already exists with local commits; force overwrite
gitee pr checkout 123 --force
```

### Common Errors

- **`本地有未提交的更改，请先提交或使用 --force 强制检出`** (*"There are uncommitted local changes, please commit first or use --force to force the checkout"*): `git stash` / `git commit` first, or pass `-f`.
- **`更新分支失败（非快进），请使用 --force 强制覆盖`** (*"Failed to update branch (non-fast-forward), please use --force to overwrite"*): the local `pr-N` and the remote PR head have diverged; confirm you want to discard local differences, then add `-f`.
- **`暂不支持通过分支名检出，请使用 PR 编号或 URL`** (*"Checking out by branch name is not supported yet, please use a PR number or URL"*): switch to a number or URL.
- **`无法从 URL 中解析 PR 编号: ...`** (*"Cannot parse PR number from URL: ..."*): the URL does not match the `https://gitee.com/<owner>/<repo>/pulls/<number>` form.

---

## issue list

List the issues of the current repository.

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--state` | `-s` | string | `open` | `open` / `closed` / `progressing` / `rejected` / `all` |
| `--author` | `-A` | string | empty | Filter by author; `@me` means the current logged-in user |
| `--assignee` | | string | empty | Filter by assignee; `@me` means the current logged-in user |
| `--label` | `-l` | string | empty | Filter by labels, comma-separated |
| `--sort` | | string | `created` | `created` / `updated` / `comments` |
| `--direction` | | string | `desc` | `asc` / `desc` |
| `--limit` | `-L` | int | `30` | 1–100 |
| `--json` | | bool | false | Output JSON |
| `--verbose` | `-v` | bool | false | Multi-column: assignee, labels, update time |
| `--no-color` | | bool | false | Disable color |

> Note: `--author` / `--assignee` are filtered client-side (the Gitee API does not support these parameters), so when combined with `--limit` the actual number returned is ≤ limit.

### Examples

```bash
# Default
gitee issue list

# Closed
gitee issue list --state closed

# Assigned to me
gitee issue list --assignee @me

# Labels + verbose
gitee issue list --label bug,urgent --verbose

# Descending by update time, output JSON
gitee issue list --sort updated --direction desc --json --limit 100

# All states
gitee issue list --state all
```

### Common Errors

- **`无效的 --state 值 "in_progress", 仅支持 open/closed/progressing/rejected/all`** (*"Invalid --state value 'in_progress', only open/closed/progressing/rejected/all are supported"*): Gitee uses `progressing` to indicate in-progress.

---

## issue view

View issue details.

### Usage

```
gitee issue view <number> [flags]
```

`<number>` is Gitee's issue number string (Gitee numbers are usually 6-character alphanumeric, e.g. `IABCDE`, not necessarily purely numeric).

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--web` | `-w` | bool | false | Open in the browser |
| `--comments` | `-c` | bool | false | Show the comment list |
| `--json` | | bool | false | Output JSON (`{issue, comments?}` structure) |
| `--no-color` | | bool | false | Disable color |

### Examples

```bash
# View an issue
gitee issue view IABCDE

# With comments
gitee issue view IABCDE --comments

# JSON
gitee issue view IABCDE --json

# Open in the browser
gitee issue view IABCDE --web
```

### Common Errors

- **`Issue 编号不能为空`** (*"Issue number cannot be empty"*): the positional argument was forgotten.
- **`查询 Issue 详情失败: ... 404`** (*"Failed to query issue details: ... 404"*): wrong number or the repository is inaccessible.

---

## ci status

View the aggregated CI/CD status for a given ref (branch / tag / commit SHA) of the repository.

### Behavior Notes

- The queried ref priority: `--ref` > `--branch` > the **current git branch**.
- Calls `GET /repos/:owner/:repo/commits/:ref/status` to fetch the aggregated status; on a 404 it gives the friendly hint "未找到 ... 的 CI 状态，请确认仓库已启用 CI/CD" (*"No CI status found for ..., please confirm the repository has CI/CD enabled"*).
- `--web` derives `https://gitee.com/<owner>/<repo>/commits/<ref>` directly from the host and opens the browser, without calling the API.

### Parameters

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--branch` | `-b` | string | current branch | Query a specific branch |
| `--ref` | | string | empty | Query a specific commit SHA or tag; takes priority over `--branch` |
| `--web` | `-w` | bool | false | Open the commits page in the browser |
| `--json` | | bool | false | Output JSON |
| `--no-color` | | bool | false | Disable color |

### Status Values

`success` / `failed` / `error` / `pending` / `running` / `canceled`.

### Examples

```bash
# Aggregated status of the latest commit on the current branch
gitee ci status

# Specific branch
gitee ci status --branch develop

# Specific commit
gitee ci status --ref abc1234

# JSON
gitee ci status --json

# Open the commits page in the browser
gitee ci status -w
```

### Common Errors

- **`未找到 owner/repo@main 的 CI 状态，请确认仓库已启用 CI/CD`** (*"No CI status found for owner/repo@main, please confirm the repository has CI/CD enabled"*): the repository has no Gitee Go or third-party CI reporting status; this is normal.
- **`查询 CI 状态失败: ... 401`** (*"Failed to query CI status: ... 401"*): the token is invalid or lacks scope; log in again.

---

## Config File and Environment Variables

| Item | Value |
| --- | --- |
| Config directory (default) | `~/.config/gitee-cli/` |
| Config file | `config.yaml` (permission `0600`) |
| Default host | `https://gitee.com/api/v5` |
| Override config directory | environment variable `GITEE_CLI_CONFIG_DIR` |

`config.yaml` fields:

```yaml
host: https://gitee.com/api/v5  # API base address
token: <your-token>              # personal access token (sensitive)
user: <login>                    # current login username (used for @me resolution)
```

> Note: when writing the token, it uses a "temp file + atomic rename" approach, strictly narrowing permission to `0600` on overwrite, to prevent leftover loose permissions from an old file.

---

## Common Errors and Troubleshooting

| Error message (prefix) | Trigger scenario | Resolution |
| --- | --- | --- |
| `未登录，请先运行 'gitee auth login' 进行认证` | Not logged in before calling any command that requires auth | `gitee auth login` |
| `加载配置失败: ...` | Config file corrupted or has abnormal permissions | Delete `~/.config/gitee-cli/config.yaml`, then log in again |
| `无法获取远程仓库 URL` / `不支持的仓库 URL 格式` | The current directory is not a git repo / origin is not Gitee | Run inside a Gitee repo directory; or use `repo view owner/repo` to target explicitly |
| `无法获取当前分支` / `当前不在任何分支上` | detached HEAD | `git checkout <branch>` to a named branch |
| `令牌校验失败（可能已过期）` | Token revoked / scope reduced | Regenerate the token, then `gitee auth login` |
| `读取密码失败（请确认在终端中运行或使用 --token 参数）` | Cannot read with no-echo in a non-TTY environment | Use `gitee auth login --token <token>` instead |
| `无效的 --state/--sort/--direction/--limit 值 ...` | Parameter enum/range validation failed | Refer to the allowed values in each command's "Parameters" table |
| `... 404` / `... 401` / `... 422` | API HTTP error | 404 = repo/number not found, 401 = auth invalid, 422 = parameter semantics error (e.g. duplicate PR) |

---

## Quick Reference

```bash
# Authentication
gitee auth login                           # interactive
gitee auth login --token $GITEE_TOKEN      # CI-friendly
gitee auth status

# Repository
gitee repo view                            # current
gitee repo view owner/repo --json

# PR
gitee pr list                              # default open
gitee pr list --state merged --author @me
gitee pr view 123 --comments
gitee pr create -t "..." -b "..."          # auto push + create
gitee pr checkout 123                      # → local pr-123

# Issue
gitee issue list --assignee @me
gitee issue view IABCDE --comments

# CI
gitee ci status                            # current branch
gitee ci status --ref abc1234 --json
```
