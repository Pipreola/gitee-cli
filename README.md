# gitee-cli

一个用于与 [Gitee（码云）](https://gitee.com) 交互的命令行工具，基于 [Gitee OpenAPI v5](https://gitee.com/api/v5/swagger) 实现。

## 状态

- ✅ 阶段一：项目基础架构搭建（已完成）
- ✅ 阶段二：认证模块实现（已完成）
- ✅ PR 创建功能（已完成）
- ✅ PR 列表功能（已完成）

当前提供项目骨架、配置管理、API 客户端封装、完整的认证功能、PR 创建与查询能力。

## 功能特性

- ✅ **认证管理**：支持交互式登录、状态查询、登出
- ✅ **PR 创建**：支持交互式和参数化创建 Pull Request
- ✅ **PR 列表**：支持按状态、作者、标签、分支过滤，输出格式与 gh/glab 一致

## 环境要求

- Go 1.19+
- Git（用于获取仓库信息和分支操作）

## 安装

```bash
go install github.com/Pipreola/gitee-cli@latest
```

或者从源码构建：

```bash
git clone https://github.com/Pipreola/gitee-cli.git
cd gitee-cli
go build -o gitee .
```

## 目录结构

```
.
├── main.go              # 程序入口
├── cmd/                 # Cobra 命令树（root / version / auth / pr）
├── pkg/
│   ├── api/             # Gitee OpenAPI v5 客户端（认证、PR、错误处理）
│   ├── config/          # 配置文件读写（~/.config/gitee-cli/config.yaml）
│   └── auth/            # 认证逻辑，桥接配置与 API
├── internal/
│   └── version/         # 版本信息
└── docs/api/            # OpenAPI 接口文档
```

## 使用

### 认证

获取私人令牌：访问 https://gitee.com/profile/personal_access_tokens 生成新令牌，建议勾选 `user_info`、`projects`、`pull_requests`、`issues` 等权限。

```bash
# 交互式登录（推荐，输入 token 不显示）
gitee auth login

# 使用参数登录
gitee auth login --token <your-personal-access-token>

# 查看当前登录状态
gitee auth status

# 登出并清除本地凭证
gitee auth logout

# 查看版本
gitee version
```

### Pull Request

```bash
# 交互式创建 PR（推荐）
gitee pr create

# 指定参数创建 PR
gitee pr create --title "Add new feature" --body "This PR adds..."

# 创建草稿 PR
gitee pr create --draft --title "WIP: Refactor auth module"

# 指定审阅者和标签
gitee pr create --title "Fix bug" --assignees user1,user2 --labels bug,urgent

# 创建后自动在浏览器中打开
gitee pr create --title "Update docs" --web
```

**参数说明**：

- `--title, -t`：PR 标题（必填，交互式时可在命令行输入）
- `--body, -b`：PR 描述（可选）
- `--base`：目标分支（默认 main 或 master）
- `--head`：源分支（默认当前分支）
- `--draft, -d`：创建为草稿 PR
- `--labels, -l`：标签，逗号分隔
- `--milestone, -m`：里程碑编号
- `--assignees, -a`：审阅者，逗号分隔的用户名
- `--testers`：测试者，逗号分隔的用户名
- `--web, -w`：在浏览器中打开创建的 PR

#### 列出 PR

```bash
# 列出当前仓库所有开放 PR（默认 --state open）
gitee pr list

# 列出已合并的 PR
gitee pr list --state merged

# 列出我创建的 PR
gitee pr list --author @me

# 按标签过滤
gitee pr list --label bug,urgent

# 按目标分支过滤
gitee pr list --base main

# 详细模式（显示作者、更新时间）
gitee pr list --verbose

# JSON 输出，便于脚本处理
gitee pr list --json --limit 50

# 按更新时间升序排序
gitee pr list --sort updated --direction asc
```

**参数说明**：

- `--state, -s`：状态过滤 open/closed/merged/all（默认 open）
- `--author, -A`：按作者用户名过滤，`@me` 表示当前登录用户
- `--label, -l`：按标签过滤，逗号分隔
- `--base`：按目标分支过滤
- `--head`：按源分支过滤（格式 namespace:branch）
- `--sort`：排序字段 created/updated/popularity/long-running（默认 created）
- `--direction`：asc/desc（默认 desc）
- `--limit, -L`：返回数量上限 1-100（默认 30）
- `--json`：输出 JSON 原始数据
- `--verbose, -v`：详细模式，显示作者与更新时间
- `--no-color`：禁用颜色输出

#### 检出 PR

将一个 Pull Request 检出到本地分支（统一命名为 `pr-<number>`），便于本地评审或测试。

```bash
# 通过 PR 编号检出
gitee pr checkout 123

# 通过 PR URL 检出
gitee pr checkout https://gitee.com/owner/repo/pulls/456

# 本地分支已存在时强制重置到 PR 最新提交（丢弃本地差异）
gitee pr checkout 123 --force
```

**参数说明**：

- `<number|url>`：PR 编号或 Gitee PR 链接（暂不支持直接通过分支名检出）
- `--force, -f`：跳过未提交更改检查，并在本地分支已存在时硬重置到 PR 最新提交

**行为说明**：

- 始终先将 PR 拉取到 `FETCH_HEAD`，再创建或更新本地分支，避免真实 git 拒绝 fetch 到当前检出分支。
- 本地分支不存在：基于 `FETCH_HEAD` 创建并切换到 `pr-<number>`。
- 本地分支已存在：切换到该分支后，非 `--force` 仅做快进更新；`--force` 硬重置到 PR 最新提交。
- 检出已关闭或已合并的 PR 时会给出提示，但仍允许检出。

配置目录可通过环境变量 `GITEE_CLI_CONFIG_DIR` 覆盖，便于测试与自定义部署。

## 配置文件

默认路径：`~/.config/gitee-cli/config.yaml`

令牌以 **0600 权限**保存，确保安全性。

```yaml
host: https://gitee.com/api/v5
token: <your-token>
user: <login>
```

## 测试

```bash
go test ./... -cover
```

当前测试覆盖率：
- `cmd`: 62.7%
- `pkg/api`: 81.7%
- `pkg/auth`: 84.6%
- `pkg/config`: 61.5%

## API 文档

接口文档位于 `docs/api/`，采用 OpenAPI 3.0 格式。当前包含：

- `user.openapi.yaml` —— 获取当前认证用户接口
- `pr.openapi.yaml` —— Pull Request 创建与查询接口

## Claude / Multica Skill

仓库同时提供一份针对 Claude / Multica 智能体的 skill 说明书，覆盖一期 9 个命令的参数、可运行示例、鉴权前置与常见错误处理：

- `.claude/skills/gitee-cli/SKILL.md`

智能体在 Gitee 仓库目录下被要求执行 PR / Issue / CI 相关任务时可直接加载该 skill。

## 许可证

MIT License
