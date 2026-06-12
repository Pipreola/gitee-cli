# Changelog

本文档记录 gitee-cli 的所有重要变更。

遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 格式，
使用 [语义化版本](https://semver.org/lang/zh-CN/) 规范。

## [Unreleased]

待发布的变更将在此处记录。

### ✨ 新增

- **`gitee pr review <number>`**（CRH-25）- 审查 Pull Request
  - `--approve` 调用 `POST /repos/{owner}/{repo}/pulls/{number}/review` 标记审查通过
  - `--comment` + `--body` / `--body-file` 仅发表审查评论
  - `--approve --body` 在审查通过后追加一条评论
  - `--force` 透传到接口 `force` 字段，忽略分支保护的审查/测试规则限制
  - 同步更新 `docs/api/pr.openapi.yaml`

---

## [1.0.0] - 2026-06-10

### 🎉 首次发布

gitee-cli v1.0.0 是第一个正式版本，提供了与 Gitee（码云）交互的核心命令行功能。

### ✨ 新增功能

#### 认证模块
- **`gitee auth login`** - 登录 Gitee 账号
  - 支持交互式输入个人访问令牌
  - 支持通过 `GITEE_TOKEN` 环境变量登录
  - 安全存储令牌（XDG 标准配置目录）

- **`gitee auth status`** - 查看当前认证状态
  - 显示当前登录用户信息
  - 验证令牌有效性

#### Pull Request 管理
- **`gitee pr create`** - 创建 Pull Request
  - 自动检测当前分支
  - 支持指定目标分支
  - 交互式填写 PR 标题和描述

- **`gitee pr list`** - 列出 Pull Request
  - 支持按状态筛选（open/merged/closed）
  - 支持按创建者筛选
  - 分页显示结果

- **`gitee pr view <number>`** - 查看 PR 详情
  - 显示完整的 PR 信息
  - 显示评论和评审状态

- **`gitee pr checkout <number>`** - 检出 PR 分支
  - 自动拉取远程分支到本地
  - 支持在 PR 分支上进行测试和评审

#### 仓库管理
- **`gitee repo view [owner/repo]`** - 查看仓库信息
  - 显示仓库基本信息（stars、forks、语言等）
  - 支持查看当前目录或指定仓库

#### Issue 管理
- **`gitee issue list`** - 列出 Issue
  - 支持按状态筛选（open/progressing/closed/rejected）
  - 支持按标签筛选
  - 分页显示

- **`gitee issue view <number>`** - 查看 Issue 详情
  - 显示完整的 Issue 信息
  - 显示评论列表

#### CI/CD 支持
- **`gitee ci status [ref]`** - 查看 CI 状态
  - 显示当前分支或指定 ref 的 CI 构建状态
  - 支持查看所有 CI 任务

### 🏗️ 技术特性

- **跨平台支持**: Linux、macOS、Windows
- **多架构支持**: amd64、arm64
- **零依赖**: 静态编译的单个可执行文件
- **配置管理**: 遵循 XDG Base Directory 规范
- **错误处理**: 友好的错误提示信息
- **API 客户端**: 基于 Gitee OpenAPI v5

### 📦 发布说明

本版本提供以下平台的预编译二进制文件：

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

可通过以下方式安装：

```bash
# 使用安装脚本（推荐）
curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash

# 或手动下载对应平台的二进制文件
```

### 🙏 致谢

感谢所有为 gitee-cli 做出贡献的开发者！

---

[Unreleased]: https://github.com/Pipreola/gitee-cli/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/Pipreola/gitee-cli/releases/tag/v1.0.0
