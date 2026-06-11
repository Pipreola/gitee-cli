# 发布指南

本文档说明如何发布 gitee-cli 的新版本。

## 发布流程

### 方式一：使用 GitHub Actions（推荐）

这是最简单的方式，适合正式发布。

1. **确保所有更改已提交并推送到 main 分支**

2. **创建并推送版本标签**
   ```bash
   # 创建标签
   git tag -a v1.0.0 -m "Release v1.0.0"

   # 推送标签
   git push origin v1.0.0
   ```

3. **GitHub Actions 自动构建和发布**
   - GitHub Actions 会自动检测到新标签
   - 运行 GoReleaser 构建所有平台的二进制文件
   - 自动创建 GitHub Release
   - 上传构建产物和校验和文件

4. **查看发布结果**
   - 访问 https://github.com/Pipreola/gitee-cli/releases
   - 确认发布成功，所有文件已上传

### 方式二：本地使用 GoReleaser

适合测试发布流程或需要手动控制的情况。

1. **确保已安装 GoReleaser**
   ```bash
   go install github.com/goreleaser/goreleaser@latest
   ```

2. **设置 GitHub Token**
   ```bash
   export GITHUB_TOKEN="your_github_personal_access_token"
   ```

3. **创建标签**
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

4. **运行 GoReleaser**
   ```bash
   # 正式发布
   make release

   # 或直接使用 goreleaser
   goreleaser release --clean
   ```

### 方式三：本地手动构建（fallback 发布）

适合 GoReleaser 不可用、或仅需本地测试的情况。`release-archives` 产出的归档命名、目录结构与 GoReleaser 完全一致，安装脚本可直接消费。

1. **构建并打包所有平台归档**（推荐，作为 GoReleaser 不可用时的 fallback）
   ```bash
   # 默认 VERSION=v1.0.0；设置 VERSION 覆盖，例如：
   VERSION=v1.2.3 make release-archives
   ```

   产物位于 `dist/`：
   - `gitee-cli_1.2.3_Linux_x86_64.tar.gz`
   - `gitee-cli_1.2.3_Linux_arm64.tar.gz`
   - `gitee-cli_1.2.3_Darwin_x86_64.tar.gz`
   - `gitee-cli_1.2.3_Darwin_arm64.tar.gz`
   - `gitee-cli_1.2.3_Windows_x86_64.zip`
   - `gitee-cli_1.2.3_Windows_arm64.zip`
   - `checksums.txt`（覆盖以上全部归档）

   命名规则与 GoReleaser 默认 `{{ .ProjectName }}_{{ .Version }}_{{ title .Os }}_{{ ARCH }}` 一致；`install.sh` 通过去掉 tag 前导 `v` 后拼接相同文件名进行下载。

2. **仅本地预览裸二进制（开发用，不用于发布）**
   ```bash
   make build-all
   make checksums
   ls -lh dist/
   ```

   产物（裸二进制，命名与 Release 资产**不一致**，仅供开发预览）：
   - `gitee-linux-amd64`、`gitee-linux-arm64`
   - `gitee-darwin-amd64`、`gitee-darwin-arm64`
   - `gitee-windows-amd64.exe`、`gitee-windows-arm64.exe`
   - `checksums.txt`

## 测试发布（快照版本）

在正式发布之前，建议先创建快照版本进行测试：

```bash
make release-snapshot
```

这会：
- 构建所有平台的二进制文件
- 生成打包文件和校验和
- **不会** 推送到 GitHub
- 产物在 `dist/` 目录中

## 验证配置

在发布之前，可以验证 GoReleaser 配置是否正确：

```bash
make release-check
```

## 版本号规范

遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范：

- **主版本号（MAJOR）**: 不兼容的 API 变更
- **次版本号（MINOR）**: 向后兼容的功能新增
- **修订号（PATCH）**: 向后兼容的问题修正

示例：
- `v1.0.0` - 首次正式发布
- `v1.1.0` - 新增功能
- `v1.1.1` - 修复 bug
- `v2.0.0` - 重大变更

## 预发布版本

对于 alpha、beta、rc 版本：

```bash
git tag -a v1.1.0-alpha.1 -m "Release v1.1.0-alpha.1"
git push origin v1.1.0-alpha.1
```

GoReleaser 会自动将其标记为 pre-release。

## 更新 CHANGELOG

在发布之前，确保更新 `CHANGELOG.md`：

1. 在 `[Unreleased]` 下记录所有更改
2. 创建新版本章节
3. 更新版本对比链接

示例：
```markdown
## [1.1.0] - 2026-06-15

### Added
- 新功能A
- 新功能B

### Fixed
- 修复问题X

[1.1.0]: https://github.com/Pipreola/gitee-cli/compare/v1.0.0...v1.1.0
```

## 发布检查清单

发布前确认：

- [ ] 所有功能测试通过
- [ ] 文档已更新
- [ ] CHANGELOG 已更新
- [ ] 版本号符合语义化版本规范
- [ ] GoReleaser 配置验证通过（`make release-check`）
- [ ] 快照版本构建成功（`make release-snapshot`）
- [ ] 已创建并推送版本标签

## 发布后

发布完成后：

1. **验证 Release 页面**
   - 检查所有平台的二进制文件
   - 确认 CHANGELOG 正确显示

2. **测试安装脚本**
   ```bash
   curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash
   ```

3. **公告发布**
   - 在项目 README 中更新版本信息
   - 发布社交媒体公告（如需要）

## 问题排查

### GoReleaser 构建失败

1. 检查 Go 版本是否正确
2. 验证配置文件：`goreleaser check`
3. 查看构建日志获取详细错误信息

### GitHub Release 创建失败

1. 确认 `GITHUB_TOKEN` 已正确设置
2. 确认 token 有 `repo` 权限
3. 确认标签已推送到远程仓库

### 平台特定构建失败

1. 检查交叉编译依赖
2. 确认 `CGO_ENABLED=0`（如果不需要 CGO）
3. 查看该平台的特定错误日志

## 发布矩阵

发布资产覆盖以下 OS×ARCH 组合（与 GoReleaser、`install.sh`、`make release-archives` 一致）：

| OS / ARCH | amd64 (x86_64) | arm64 |
|-----------|----------------|-------|
| Linux     | ✅              | ✅     |
| Darwin    | ✅              | ✅     |
| Windows   | ✅ (.zip)       | ✅ (.zip) |

> Windows arm64 自 Go 1.17 起原生支持，`install.sh` 在 Windows arm64 主机上可正常下载对应 zip。

## install.sh 下载 URL 拼接验证

为避免 GoReleaser 产物名（`{{ .Version }}` 不含前导 `v`）与 `install.sh` 拼接（基于 release `tag_name`，含 `v`）出现不一致导致 404，仓库提供了拼接验证脚本：

```bash
bash scripts/test-install-url.sh
```

该脚本对所有平台（含 Windows arm64）和预发布 tag（如 `v1.1.0-rc.1`）做 URL 拼接断言，期望产出与 GoReleaser 默认归档命名一致：

```
https://github.com/Pipreola/gitee-cli/releases/download/v1.0.0/gitee-cli_1.0.0_Linux_x86_64.tar.gz
                                                       ^tag(含v)         ^version(去v)
```

发布前请连同 `make release-check`、`make release-snapshot` 一并执行，确认 install/release 链路对齐。
