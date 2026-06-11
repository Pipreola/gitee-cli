---
name: gitee-cli
description: gitee CLI 一期命令使用说明书。覆盖 auth login/status、repo view、pr list/view/create/checkout、issue list/view、ci status 共 9 个命令的参数、可运行示例、鉴权前置与常见错误处理。当用户在 Gitee（码云）仓库下需要查看仓库信息、查询 / 创建 / 检出 PR、查询 Issue 或查看 CI 状态时调用此 skill。
---

# gitee CLI 使用说明书（一期）

`gitee` 是仿照 `gh` / `glab` 风格、基于 [Gitee OpenAPI v5](https://gitee.com/api/v5/swagger) 实现的命令行工具。本文档以**已合并的代码为准**，覆盖一期全部 9 个命令。

## 目录

- [鉴权前置（必读）](#鉴权前置必读)
- [auth login](#auth-login) / [auth status](#auth-status) / [auth logout](#auth-logout附属)
- [repo view](#repo-view)
- [pr list](#pr-list) / [pr view](#pr-view) / [pr create](#pr-create) / [pr checkout](#pr-checkout)
- [issue list](#issue-list) / [issue view](#issue-view)
- [ci status](#ci-status)
- [配置文件与环境变量](#配置文件与环境变量)
- [常见错误与排查](#常见错误与排查)

---

## 鉴权前置（必读）

除 `gitee auth login` / `gitee version` 外，**所有命令都要求本地已登录**（配置中已保存合法 token）。未登录时命令会直接报错：

```
未登录，请先运行 'gitee auth login' 进行认证
```

### Token 从哪来

访问 https://gitee.com/profile/personal_access_tokens 创建私人令牌（personal access token），建议勾选以下权限：

- `user_info` —— `auth status`、`pr list --author @me` 等需要识别当前用户
- `projects` —— `repo view`
- `pull_requests` —— `pr list/view/create/checkout`
- `issues` —— `issue list/view`

### Token 配置在哪

`gitee auth login` 校验 token 后会写入 **`~/.config/gitee-cli/config.yaml`**（权限 `0600`）。文件内容形如：

```yaml
host: https://gitee.com/api/v5
token: <your-token>
user: <your-login>
```

可通过环境变量 `GITEE_CLI_CONFIG_DIR` 覆盖配置目录（用于测试或多账号切换）。

### 仓库定位

除 `repo view [owner/repo]` 显式接受参数外，其他命令都依赖 **当前目录的 git remote `origin`** 推断 `owner/repo`。支持两种远程地址格式：

- `https://gitee.com/owner/repo.git`
- `git@gitee.com:owner/repo.git`

不在 git 仓库目录、或 origin 指向非 Gitee 域名时，`pr` / `issue` / `ci` 类命令会失败。

---

## auth login

使用私人令牌登录 Gitee，校验通过后写入本地配置。

### 参数

| Flag | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `--token` | string | 空 | Gitee 私人令牌；留空则进入交互式无回显输入 |
| `--host` | string | `https://gitee.com/api/v5` | Gitee API 基础地址（私有化部署时使用） |

### 示例

```bash
# 推荐：交互式输入，token 不会回显到终端
gitee auth login

# 直接通过参数传入（CI 场景）
gitee auth login --token <your-personal-access-token>

# 私有化部署
gitee auth login --token <token> --host https://gitee.example.com/api/v5
```

成功输出：

```
✓ 登录成功，当前用户: 张三 (zhangsan)
```

### 常见错误

- **`读取密码失败（请确认在终端中运行或使用 --token 参数）`**：在非 TTY 环境（管道、CI、IDE 集成终端）执行 `gitee auth login` 而未带 `--token`。改用 `gitee auth login --token <token>`。
- **`令牌不能为空`**：交互式输入时直接回车，或粘贴的内容被解释为空。
- **`令牌校验失败: ...`**：token 已过期、被撤销或权限不足。重新去 https://gitee.com/profile/personal_access_tokens 生成新 token。

## auth status

查看当前登录状态。

### 参数

无。

### 示例

```bash
gitee auth status
```

已登录输出：

```
✓ 已登录
用户: zhangsan
主页: https://gitee.com/zhangsan
```

未登录输出：

```
✗ 未登录
使用 'gitee auth login' 命令登录
```

### 常见错误

- **`令牌校验失败（可能已过期）: ...`**：本地保存的 token 仍在但已被服务端撤销。运行 `gitee auth logout` 后重新 `gitee auth login`。

## auth logout（附属）

> 不在一期 9 命令清单内，但与 `auth login/status` 同属 `auth` 命令组，已实现并可用。

清除本地凭证。

```bash
gitee auth logout
# ✓ 已登出，本地凭证已清除
```

---

## repo view

查看仓库的基本信息与统计数据。

### 用法

```
gitee repo view [owner/repo] [flags]
```

不带位置参数时从当前目录的 `origin` 远程地址推断仓库。

### 参数

| Flag / 位置参数 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `[owner/repo]` | string | 当前 git remote | 显式定位任意仓库，例如 `oschina/git-osc` |
| `--web` / `-w` | bool | false | 不打印详情，直接在浏览器中打开仓库主页 |
| `--json` | bool | false | 输出 OpenAPI 原始 JSON |
| `--no-color` | bool | false | 禁用 ANSI 颜色（管道场景 stdout 非 TTY 时自动禁用） |

### 示例

```bash
# 查看当前仓库
gitee repo view

# 查看指定仓库
gitee repo view oschina/git-osc

# 输出 JSON 便于 jq 处理
gitee repo view --json | jq '.stargazers_count'

# 在浏览器中打开
gitee repo view -w
```

### 常见错误

- **`无效的仓库定位 "...", 期望格式 owner/repo`**：位置参数缺少 `/` 或某一段为空。
- **`获取仓库信息失败: ... 404`**：仓库不存在、无权访问或已删除。

---

## pr list

列出当前仓库的 Pull Request，输出格式对齐 `gh pr list` / `glab mr list`。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--state` | `-s` | string | `open` | `open` / `closed` / `merged` / `all` |
| `--author` | `-A` | string | 空 | 按作者用户名过滤；`@me` 表示当前登录用户（客户端过滤） |
| `--label` | `-l` | string | 空 | 按标签过滤，逗号分隔，如 `bug,urgent` |
| `--base` | | string | 空 | 按目标分支过滤 |
| `--head` | | string | 空 | 按源分支过滤，格式 `namespace:branch` |
| `--sort` | | string | `created` | `created` / `updated` / `popularity` / `long-running` |
| `--direction` | | string | `desc` | `asc` / `desc` |
| `--limit` | `-L` | int | `30` | 1–100 |
| `--json` | | bool | false | 输出 JSON 原始数组 |
| `--verbose` | `-v` | bool | false | 多列显示作者、更新时间 |
| `--no-color` | | bool | false | 禁用颜色 |

### 示例

```bash
# 默认：当前仓库所有 open PR
gitee pr list

# 已合并 + 更新时间倒序
gitee pr list --state merged --sort updated --direction desc

# 我的 PR（已登录后可用 @me）
gitee pr list --author @me

# 多标签过滤 + 详细模式
gitee pr list --label bug,urgent --verbose

# 输出 JSON 给脚本处理
gitee pr list --state all --limit 100 --json | jq '.[].number'

# 按目标分支过滤
gitee pr list --base main
```

### 常见错误

- **`无效的 --state 值 "draft"，仅支持 open/closed/merged/all`**：Gitee v5 不区分 draft 状态，使用 `open` 后在结果中筛选。
- **`无效的 --limit 值 200，须在 1-100 之间`**：单次调用上限即为 100，需分页脚本化处理。
- **`无法解析 @me：本地配置未保存登录用户名，请重新执行 'gitee auth login'`**：早期登录的 config 缺 `user` 字段，重新登录即可。

---

## pr view

查看指定 PR 的详细信息。

### 用法

```
gitee pr view <number> [flags]
```

`<number>` 必须是正整数。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--web` | `-w` | bool | false | 打开 PR 页面（仍会先获取 PR 详情以取得 URL；不会拉取评论或输出详情） |
| `--comments` | `-c` | bool | false | 追加显示 PR 评论列表 |
| `--json` | | bool | false | 输出 JSON（`{pull_request, comments?}` 结构） |
| `--no-color` | | bool | false | 禁用颜色 |

### 示例

```bash
# 查看 PR #123
gitee pr view 123

# 同时显示评论
gitee pr view 123 --comments

# JSON 输出（含评论）
gitee pr view 123 --comments --json

# 直接打开浏览器
gitee pr view 123 -w
```

### 常见错误

- **`无效的 PR 编号 "abc"，必须是正整数`**：参数应该是数字，URL 形式只在 `pr checkout` 中支持。
- **`获取 PR 详情失败: ... 404`**：PR 编号不存在或仓库无权访问。

---

## pr create

创建一个新的 Pull Request。未指定标题/描述时进入交互式提示。

### 行为要点（来自代码）

- `--head` 默认取**当前分支**；`--base` 默认取 `origin/main`，不存在时回退 `origin/master`，再不存在则报错要求显式 `--base`。
- 创建前会检查 `--head` 分支是否已推送到 origin，未推送则**自动 `git push -u origin <branch>`**。
- `head == base` 直接报错。
- 未指定 `--body` 且 `--body` 标志未被显式置过（即使是 `""`），会进入交互式描述输入；显式 `--body ""` 会被尊重为空描述、不再询问。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--title` | `-t` | string | 交互式 | PR 标题；交互式输入时不能为空 |
| `--body` | `-b` | string | 交互式 | PR 描述，可选 |
| `--base` | | string | 自动 | 目标分支，默认 `main` 或 `master` |
| `--head` | | string | 当前分支 | 源分支 |
| `--draft` | `-d` | bool | false | 创建为草稿 PR |
| `--labels` | `-l` | string | 空 | 标签，逗号分隔 |
| `--milestone` | `-m` | int64 | 0 | 里程碑编号；0 表示不设置 |
| `--assignees` | `-a` | string | 空 | 审阅者用户名，逗号分隔 |
| `--testers` | | string | 空 | 测试者用户名，逗号分隔 |
| `--web` | `-w` | bool | false | 创建后在浏览器中打开 |

### 示例

```bash
# 完全交互式创建
gitee pr create

# 一行创建
gitee pr create --title "Add new feature" --body "This PR adds ..."

# 草稿 PR
gitee pr create --draft --title "WIP: Refactor auth module"

# 指定审阅者 + 标签
gitee pr create --title "Fix bug" \
  --assignees user1,user2 \
  --labels bug,urgent

# 自定义源/目标分支
gitee pr create --title "Cherry pick to release" \
  --head feature/x --base release/1.0

# 关联里程碑并自动开浏览器
gitee pr create -t "Update docs" -m 42 -w
```

### 常见错误

- **`源分支和目标分支不能相同（当前都是 main）`**：当前分支就是 `main`，需要先切到功能分支。
- **`找不到默认分支（main 或 master），请使用 --base 指定目标分支`**：远程没有 `origin/main` 也没有 `origin/master`，显式 `--base release/1.0` 之类。
- **`分支 'feature/x' 尚未推送到远程，正在推送...`** 后跟 `推送分支失败`：通常是远程权限或网络问题，先 `git push -u origin feature/x` 手动验证。
- **`PR 标题不能为空`**：交互式输入时直接回车。
- **`创建 PR 失败: ... 422`**：常见原因——已存在同源/目标分支的 open PR、目标仓库未开启 PR、源分支没有新 commit。

---

## pr checkout

将一个 PR 检出到本地分支，本地分支统一命名 `pr-<number>`。

### 用法

```
gitee pr checkout <number|url> [flags]
```

支持两种输入：

- 纯数字 PR 编号，如 `123`
- Gitee PR URL，如 `https://gitee.com/owner/repo/pulls/456`

> **暂不支持**通过分支名检出（代码注释明确说明：需要额外的 list PR API 才能定位编号）。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--force` | `-f` | bool | false | 跳过未提交更改检查；本地分支已存在时硬重置到 PR 最新提交（丢弃本地差异） |

### 行为说明

1. 始终先 `git fetch origin pull/<number>/head` 拉到 `FETCH_HEAD`，避免直接 fetch 到当前检出分支被拒绝。
2. 本地 `pr-<number>` 不存在 → `git checkout -b pr-<number> FETCH_HEAD`。
3. 本地 `pr-<number>` 已存在 → 切换到该分支：
   - 默认尝试 `git merge --ff-only`，非快进时报错；
   - `--force` 改为 `git reset --hard FETCH_HEAD`。
4. 检出 closed / merged 的 PR 会有警告但仍允许检出。

### 示例

```bash
# 通过编号检出
gitee pr checkout 123

# 通过 URL 检出
gitee pr checkout https://gitee.com/owner/repo/pulls/456

# 已有 pr-123 且本地有 commit，强制覆盖
gitee pr checkout 123 --force
```

### 常见错误

- **`本地有未提交的更改，请先提交或使用 --force 强制检出`**：先 `git stash` / `git commit`，或带 `-f`。
- **`更新分支失败（非快进），请使用 --force 强制覆盖`**：本地 `pr-N` 与远端 PR 头分歧，确认要丢弃本地差异后加 `-f`。
- **`暂不支持通过分支名检出，请使用 PR 编号或 URL`**：换成编号或 URL。
- **`无法从 URL 中解析 PR 编号: ...`**：URL 不符合 `https://gitee.com/<owner>/<repo>/pulls/<number>` 形式。

---

## issue list

列出当前仓库的 Issue。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--state` | `-s` | string | `open` | `open` / `closed` / `progressing` / `rejected` / `all` |
| `--author` | `-A` | string | 空 | 按作者过滤；`@me` 表示当前登录用户 |
| `--assignee` | | string | 空 | 按指派人过滤；`@me` 表示当前登录用户 |
| `--label` | `-l` | string | 空 | 按标签过滤，逗号分隔 |
| `--sort` | | string | `created` | `created` / `updated` / `comments` |
| `--direction` | | string | `desc` | `asc` / `desc` |
| `--limit` | `-L` | int | `30` | 1–100 |
| `--json` | | bool | false | 输出 JSON |
| `--verbose` | `-v` | bool | false | 多列：指派人、标签、更新时间 |
| `--no-color` | | bool | false | 禁用颜色 |

> 注意：`--author` / `--assignee` 在客户端过滤（Gitee 接口不支持这些参数），所以与 `--limit` 组合时实际返回数 ≤ limit。

### 示例

```bash
# 默认
gitee issue list

# 已关闭
gitee issue list --state closed

# 指派给我的
gitee issue list --assignee @me

# 标签 + 详细
gitee issue list --label bug,urgent --verbose

# 按更新时间倒序，输出 JSON
gitee issue list --sort updated --direction desc --json --limit 100

# 全部状态
gitee issue list --state all
```

### 常见错误

- **`无效的 --state 值 "in_progress", 仅支持 open/closed/progressing/rejected/all`**：Gitee 用 `progressing` 表示进行中。

---

## issue view

查看 Issue 详情。

### 用法

```
gitee issue view <number> [flags]
```

`<number>` 是 Gitee 的 Issue 编号字符串（Gitee 编号通常是 6 位字母数字混合，如 `IABCDE`，不一定是纯数字）。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--web` | `-w` | bool | false | 在浏览器中打开 |
| `--comments` | `-c` | bool | false | 显示评论列表 |
| `--json` | | bool | false | 输出 JSON（`{issue, comments?}` 结构） |
| `--no-color` | | bool | false | 禁用颜色 |

### 示例

```bash
# 查看 Issue
gitee issue view IABCDE

# 含评论
gitee issue view IABCDE --comments

# JSON
gitee issue view IABCDE --json

# 浏览器打开
gitee issue view IABCDE --web
```

### 常见错误

- **`Issue 编号不能为空`**：忘记传位置参数。
- **`查询 Issue 详情失败: ... 404`**：编号错误或仓库无权访问。

---

## ci status

查看仓库指定 ref（分支 / tag / commit SHA）的 CI/CD 聚合状态。

### 行为要点

- 查询的 ref 优先级：`--ref` > `--branch` > **当前 git 分支**。
- 调用 `GET /repos/:owner/:repo/commits/:ref/status` 拉取聚合状态；返回 404 时给出"未找到 ... 的 CI 状态，请确认仓库已启用 CI/CD"的友好提示。
- `--web` 直接根据 host 推导出 `https://gitee.com/<owner>/<repo>/commits/<ref>` 并打开浏览器，不再调用 API。

### 参数

| Flag | 简写 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `--branch` | `-b` | string | 当前分支 | 查询指定分支 |
| `--ref` | | string | 空 | 查询指定 commit SHA 或 tag；优先级高于 `--branch` |
| `--web` | `-w` | bool | false | 在浏览器中打开 commits 页 |
| `--json` | | bool | false | 输出 JSON |
| `--no-color` | | bool | false | 禁用颜色 |

### 状态值

`success` / `failed` / `error` / `pending` / `running` / `canceled`。

### 示例

```bash
# 当前分支最新 commit 的聚合状态
gitee ci status

# 指定分支
gitee ci status --branch develop

# 指定 commit
gitee ci status --ref abc1234

# JSON
gitee ci status --json

# 浏览器打开 commits 页
gitee ci status -w
```

### 常见错误

- **`未找到 owner/repo@main 的 CI 状态，请确认仓库已启用 CI/CD`**：仓库没接 Gitee Go 或第三方 CI 上报状态，属正常情况。
- **`查询 CI 状态失败: ... 401`**：token 失效或权限不足，重登。

---

## 配置文件与环境变量

| 项 | 值 |
| --- | --- |
| 配置目录（默认） | `~/.config/gitee-cli/` |
| 配置文件 | `config.yaml`（权限 `0600`） |
| 默认 host | `https://gitee.com/api/v5` |
| 覆盖配置目录 | 环境变量 `GITEE_CLI_CONFIG_DIR` |

`config.yaml` 字段：

```yaml
host: https://gitee.com/api/v5  # API 基础地址
token: <your-token>              # 私人令牌（敏感）
user: <login>                    # 当前登录用户名（用于 @me 解析）
```

> 注意：写入 token 时使用"临时文件 + 原子 rename"方式，覆盖时严格收敛权限到 `0600`，防止旧文件遗留宽松权限。

---

## 常见错误与排查

| 错误信息（前缀） | 触发场景 | 处理方式 |
| --- | --- | --- |
| `未登录，请先运行 'gitee auth login' 进行认证` | 调用任何需鉴权命令前未登录 | `gitee auth login` |
| `加载配置失败: ...` | 配置文件损坏或权限异常 | 删除 `~/.config/gitee-cli/config.yaml` 后重新登录 |
| `无法获取远程仓库 URL` / `不支持的仓库 URL 格式` | 当前目录不是 git 仓库 / origin 不是 Gitee | 在 Gitee 仓库目录下运行；或使用 `repo view owner/repo` 显式定位 |
| `无法获取当前分支` / `当前不在任何分支上` | detached HEAD | `git checkout <branch>` 切到具名分支 |
| `令牌校验失败（可能已过期）` | token 被撤销 / 权限缩减 | 重新生成 token 后 `gitee auth login` |
| `读取密码失败（请确认在终端中运行或使用 --token 参数）` | 非 TTY 环境无法无回显读取 | 改用 `gitee auth login --token <token>` |
| `无效的 --state/--sort/--direction/--limit 值 ...` | 参数枚举/范围校验失败 | 参考各命令"参数"表中的允许值 |
| `... 404` / `... 401` / `... 422` | API HTTP 错误 | 404 = 仓库/编号不存在，401 = 鉴权失效，422 = 参数语义错误（如重复 PR） |

---

## 速查表

```bash
# 鉴权
gitee auth login                           # 交互式
gitee auth login --token $GITEE_TOKEN      # CI 友好
gitee auth status

# 仓库
gitee repo view                            # 当前
gitee repo view owner/repo --json

# PR
gitee pr list                              # 默认 open
gitee pr list --state merged --author @me
gitee pr view 123 --comments
gitee pr create -t "..." -b "..."          # 自动 push + 创建
gitee pr checkout 123                      # → 本地 pr-123

# Issue
gitee issue list --assignee @me
gitee issue view IABCDE --comments

# CI
gitee ci status                            # 当前分支
gitee ci status --ref abc1234 --json
```
