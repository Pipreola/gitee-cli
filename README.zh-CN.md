# gitee-cli

[English](README.md) | **简体中文**

一个用于与 [Gitee（码云）](https://gitee.com) 交互的命令行工具，基于 [Gitee OpenAPI v5](https://gitee.com/api/v5/swagger) 实现。

在终端中完成认证、Pull Request、Issue、仓库与 CI 状态管理，输出风格与 `gh` / `glab` 保持一致。

## 状态

- ✅ 阶段一：项目基础架构搭建（已完成）
- ✅ 阶段一：认证模块实现（已完成）
- ✅ PR 创建 / 列表 / 详情 / 检出（已完成）
- ✅ Issue 列表 / 详情（已完成）
- ✅ CI 状态查询（已完成）

当前版本提供项目骨架、配置管理、API 客户端封装、完整认证，以及 PR / Issue / Repo / CI 能力。

## 功能特性

一期发布覆盖以下命令：

| 命令 | 说明 |
| --- | --- |
| `gitee auth login` | 交互式或令牌登录 |
| `gitee auth status` | 查看当前登录状态 |
| `gitee auth logout` | 登出并清除本地凭证 |
| `gitee repo view` | 查看仓库信息 |
| `gitee pr create` | 创建 Pull Request（交互式或参数化） |
| `gitee pr list` | 按状态/作者/标签/分支过滤列出 PR |
| `gitee pr view` | 查看 Pull Request 详情 |
| `gitee pr checkout` | 将 PR 检出到本地分支 |
| `gitee issue list` | 按条件过滤列出 Issue |
| `gitee issue view` | 查看 Issue 详情 |
| `gitee ci status` | 查看仓库的 CI/CD 构建状态 |

输出风格与 `gh` / `glab` 一致，多数查询命令支持 `--json` 便于脚本处理。

## 环境要求

- Go 1.25+（仅在从源码构建或使用 `go install` 时需要）
- Git（用于获取仓库信息和分支操作）

## 快速开始

```bash
# 1. 安装（macOS / Linux）
curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash

# 2. 登录（在 https://gitee.com/profile/personal_access_tokens 生成令牌）
gitee auth login

# 3. 验证
gitee auth status

# 4. 基于当前分支创建 PR
gitee pr create --title "Add new feature" --body "This PR adds ..."
```

## 安装

### 一键安装脚本（推荐）

**macOS / Linux / Git Bash / WSL：**

```bash
curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash
```

**Windows PowerShell：**

```powershell
irm https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.ps1 | iex
```

PowerShell 脚本会自动检测架构（x64 / arm64），从 GitHub Releases 下载对应版本的 `gitee.exe`，安装到 `%LOCALAPPDATA%\Programs\gitee` 并加入用户 `PATH`。安装后重新打开终端即可使用 `gitee`。

> 如遇执行策略限制，可在当前会话临时放开：
> `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`

### 手动下载

1. 从 [最新 Release](https://github.com/Pipreola/gitee-cli/releases/latest) 下载对应平台的压缩包：
   - Linux / macOS：`gitee_<版本>_<系统>_<架构>.tar.gz`
   - Windows：`gitee_<版本>_Windows_<架构>.zip`
2. 解压得到 `gitee`（或 `gitee.exe`）可执行文件。
3. 将其移动到 `PATH` 中的目录：
   - macOS / Linux：`sudo mv gitee /usr/local/bin/`
   - Windows：将 `gitee.exe` 放到如 `%LOCALAPPDATA%\Programs\gitee` 的目录，并把该目录加入用户 `PATH`。
4. 验证：`gitee version`

### 使用 go install

```bash
go install github.com/Pipreola/gitee-cli@latest
```

### 从源码构建

```bash
git clone https://github.com/Pipreola/gitee-cli.git
cd gitee-cli
go build -o gitee .
```

## 目录结构

```
.
├── main.go              # 程序入口
├── cmd/                 # Cobra 命令树（root / version / auth / pr / issue / repo / ci）
├── pkg/
│   ├── api/             # Gitee OpenAPI v5 客户端（认证、PR、Issue、Repo、CI、错误处理）
│   ├── config/          # 配置文件读写（~/.config/gitee-cli/config.yaml）
│   └── auth/            # 认证逻辑，桥接配置与 API
├── internal/
│   └── version/         # 版本信息
└── docs/api/            # OpenAPI 接口文档
```

## 使用

### 认证

在 https://gitee.com/profile/personal_access_tokens 生成私人令牌，建议勾选 `user_info`、`projects`、`pull_requests`、`issues` 等权限。

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
# 交互式创建（推荐）
gitee pr create

# 指定参数创建
gitee pr create --title "Add new feature" --body "This PR adds ..."

# 创建草稿 PR
gitee pr create --draft --title "WIP: Refactor auth module"

# 指定审阅者和标签
gitee pr create --title "Fix bug" --assignees user1,user2 --labels bug,urgent

# 创建后自动在浏览器中打开
gitee pr create --title "Update docs" --web

# 列出当前仓库所有开放 PR（默认 --state open）
gitee pr list

# 列出已合并的 PR / 我创建的 PR
gitee pr list --state merged
gitee pr list --author @me

# JSON 输出，便于脚本处理
gitee pr list --json --limit 50

# 查看 PR 详情（含评论）
gitee pr view 123 --comments

# 将 PR 检出到本地分支（统一命名为 pr-<number>）
gitee pr checkout 123
gitee pr checkout https://gitee.com/owner/repo/pulls/456
```

### Issue

```bash
# 列出 Issue（可按指派人、状态、标签等过滤）
gitee issue list
gitee issue list --assignee @me

# 查看 Issue 详情（含评论）
gitee issue view IABCDE --comments
```

### 仓库与 CI

```bash
# 查看仓库信息（当前仓库或 owner/repo）
gitee repo view
gitee repo view owner/repo --json

# 查看当前分支的 CI/CD 状态
gitee ci status
gitee ci status --ref abc1234 --json
```

任意命令均可通过 `gitee <command> --help` 查看完整参数列表。

## 配置文件

默认路径：`~/.config/gitee-cli/config.yaml`。令牌以 **0600 权限**保存。

```yaml
host: https://gitee.com/api/v5
token: <your-token>
user: <login>
```

配置目录可通过环境变量 `GITEE_CLI_CONFIG_DIR` 覆盖，便于测试与自定义部署。

## 测试

```bash
go test ./... -cover
```

## API 文档

接口文档位于 `docs/api/`，采用 OpenAPI 3.0 格式，当前包含：`user`、`pr`、`issue`、`repo`、`ci`。

## Claude / Multica Skill

仓库同时提供一份针对 Claude / Multica 智能体的 skill 说明书，覆盖一期全部命令的参数、可运行示例、鉴权前置与常见错误处理：

- `.claude/skills/gitee-cli/SKILL.md`

## 贡献指南

欢迎提交 Issue 与 Pull Request。提交前请运行 `go test ./...` 与 `gofmt`，并在提交信息中关联相关 Issue。版本历史见 [CHANGELOG.md](CHANGELOG.md)。

## 许可证

基于 [Apache License 2.0](LICENSE) 许可证开源。
