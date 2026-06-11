# Makefile for gitee-cli
# 提供本地构建和发布管理功能
#
# 注: `make build` 仅用于开发；正式发布 fallback 路径请使用 `make release-archives`，
# 它生成的归档命名与 checksums 与 GoReleaser/install.sh 完全一致。

# 变量定义
VERSION ?= v1.0.0
# GoReleaser 的 {{ .Version }} 不含前导 v，归档命名/checksums 与之对齐
VERSION_NO_V := $(patsubst v%,%,$(VERSION))
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY ?= make

BINARY_NAME := gitee
MAIN_PATH := ./main.go

# 编译标志
LDFLAGS := -s -w \
	-X main.versionStr=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE) \
	-X main.builtBy=$(BUILT_BY)

# 输出目录
DIST_DIR := dist

# 颜色输出
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m # No Color

# 默认目标
.DEFAULT_GOAL := help

# 显示帮助信息
.PHONY: help
help:
	@echo "$(BLUE)gitee-cli Makefile 命令列表$(NC)"
	@echo ""
	@echo "$(GREEN)开发命令:$(NC)"
	@echo "  make build          - 构建当前平台的二进制文件（开发用）"
	@echo "  make install        - 安装到本地 \$$GOPATH/bin"
	@echo "  make test           - 运行测试"
	@echo "  make clean          - 清理构建产物"
	@echo ""
	@echo "$(GREEN)发布命令:$(NC)"
	@echo "  make build-all      - 交叉编译所有平台的裸二进制（开发预览）"
	@echo "  make release-archives - 打包所有平台归档(.tar.gz/.zip) + checksums，"
	@echo "                         产物名与 GoReleaser/install.sh 一致（fallback 发布路径）"
	@echo "  make release        - 使用 GoReleaser 创建发布"
	@echo "  make release-snapshot - 创建快照版本（不发布到 GitHub）"
	@echo ""
	@echo "$(GREEN)维护命令:$(NC)"
	@echo "  make fmt            - 格式化代码"
	@echo "  make lint           - 代码检查"
	@echo "  make tidy           - 整理依赖"
	@echo ""

# 构建当前平台
.PHONY: build
build:
	@echo "$(BLUE)构建 $(BINARY_NAME) ($(VERSION))...$(NC)"
	@mkdir -p $(DIST_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)构建完成: $(DIST_DIR)/$(BINARY_NAME)$(NC)"

# 安装到本地
.PHONY: install
install:
	@echo "$(BLUE)安装 $(BINARY_NAME)...$(NC)"
	go install -ldflags "$(LDFLAGS)" $(MAIN_PATH)
	@echo "$(GREEN)安装完成$(NC)"

# 运行测试
.PHONY: test
test:
	@echo "$(BLUE)运行测试...$(NC)"
	go test -v -race -coverprofile=coverage.out ./...
	@echo "$(GREEN)测试完成$(NC)"

# 代码覆盖率
.PHONY: coverage
coverage: test
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)覆盖率报告: coverage.html$(NC)"

# 清理构建产物
.PHONY: clean
clean:
	@echo "$(YELLOW)清理构建产物...$(NC)"
	rm -rf $(DIST_DIR)
	rm -f coverage.out coverage.html
	@echo "$(GREEN)清理完成$(NC)"

# 格式化代码
.PHONY: fmt
fmt:
	@echo "$(BLUE)格式化代码...$(NC)"
	go fmt ./...
	@echo "$(GREEN)格式化完成$(NC)"

# 代码检查
.PHONY: lint
lint:
	@echo "$(BLUE)代码检查...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)警告: golangci-lint 未安装，跳过检查$(NC)"; \
		echo "安装方法: https://golangci-lint.run/usage/install/"; \
	fi

# 整理依赖
.PHONY: tidy
tidy:
	@echo "$(BLUE)整理依赖...$(NC)"
	go mod tidy
	@echo "$(GREEN)依赖整理完成$(NC)"

# 构建所有平台（开发预览：裸二进制，不用于正式发布）
.PHONY: build-all
build-all: clean
	@echo "$(BLUE)构建所有平台的二进制文件（开发预览）...$(NC)"
	@echo "$(YELLOW)注意: 此目标产出裸二进制，仅供本地预览。"
	@echo "      正式发布 fallback 请使用 'make release-archives'。$(NC)"
	@mkdir -p $(DIST_DIR)

	# Linux amd64
	@echo "$(BLUE)构建 Linux amd64...$(NC)"
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

	# Linux arm64
	@echo "$(BLUE)构建 Linux arm64...$(NC)"
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)

	# macOS amd64
	@echo "$(BLUE)构建 macOS amd64...$(NC)"
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)

	# macOS arm64
	@echo "$(BLUE)构建 macOS arm64...$(NC)"
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

	# Windows amd64
	@echo "$(BLUE)构建 Windows amd64...$(NC)"
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

	# Windows arm64
	@echo "$(BLUE)构建 Windows arm64...$(NC)"
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PATH)

	@echo "$(GREEN)所有平台构建完成$(NC)"
	@ls -lh $(DIST_DIR)

# 生成校验和（覆盖 dist 目录中的所有产物）
.PHONY: checksums
checksums:
	@echo "$(BLUE)生成校验和...$(NC)"
	@if [ ! -d "$(DIST_DIR)" ] || [ -z "$$(ls -A $(DIST_DIR) 2>/dev/null)" ]; then \
		echo "$(YELLOW)错误: $(DIST_DIR) 为空，请先运行 build-all 或 release-archives$(NC)"; \
		exit 1; \
	fi
	@cd $(DIST_DIR) && \
		ls | grep -v '^checksums\.txt$$' | xargs -I{} sha256sum "{}" > checksums.txt 2>/dev/null || \
		(cd $(DIST_DIR) && ls | grep -v '^checksums\.txt$$' | xargs -I{} shasum -a 256 "{}" > checksums.txt)
	@echo "$(GREEN)校验和已生成: $(DIST_DIR)/checksums.txt$(NC)"

# 发布 fallback：本地打包归档，命名/checksums 与 GoReleaser、install.sh 一致
# 归档结构: <dist>/gitee-cli_<VERSION_NO_V>_<OS>_<ARCH>.tar.gz|zip，内含 gitee[.exe]、README.md、LICENSE、CHANGELOG.md
# Windows zip 优先使用 'zip'，缺失时回退到 'python3 -m zipfile'
.PHONY: release-archives
release-archives: clean
	@echo "$(BLUE)打包发布归档 (version=$(VERSION_NO_V))...$(NC)"
	@command -v zip >/dev/null 2>&1 || command -v python3 >/dev/null 2>&1 || { \
		echo "$(YELLOW)错误: 打包 Windows zip 需要 'zip' 或 'python3' 任一存在$(NC)"; exit 1; }
	@mkdir -p $(DIST_DIR)
	@$(MAKE) --no-print-directory _archive GOOS=linux   GOARCH=amd64 OS_NAME=Linux   ARCH_NAME=x86_64 EXT=tar.gz
	@$(MAKE) --no-print-directory _archive GOOS=linux   GOARCH=arm64 OS_NAME=Linux   ARCH_NAME=arm64  EXT=tar.gz
	@$(MAKE) --no-print-directory _archive GOOS=darwin  GOARCH=amd64 OS_NAME=Darwin  ARCH_NAME=x86_64 EXT=tar.gz
	@$(MAKE) --no-print-directory _archive GOOS=darwin  GOARCH=arm64 OS_NAME=Darwin  ARCH_NAME=arm64  EXT=tar.gz
	@$(MAKE) --no-print-directory _archive GOOS=windows GOARCH=amd64 OS_NAME=Windows ARCH_NAME=x86_64 EXT=zip
	@$(MAKE) --no-print-directory _archive GOOS=windows GOARCH=arm64 OS_NAME=Windows ARCH_NAME=arm64  EXT=zip
	@$(MAKE) --no-print-directory checksums
	@echo "$(GREEN)发布归档完成: $(DIST_DIR)$(NC)"
	@ls -lh $(DIST_DIR)

# 内部目标：交叉编译并打包单个平台归档
# 输入: GOOS GOARCH OS_NAME ARCH_NAME EXT
# 注: shell block 启用 set -e，并在打包前显式检查二进制存在且非空，
#     避免 go build 失败（例如 GOCACHE 只读）时仍生成无二进制的坏归档。
.PHONY: _archive
_archive:
	@echo "$(BLUE)打包 $(OS_NAME)/$(ARCH_NAME) ($(EXT))...$(NC)"
	@set -e; \
	stage="$(DIST_DIR)/_stage_$(GOOS)_$(GOARCH)"; \
	rm -rf "$$stage"; mkdir -p "$$stage"; \
	bin="$(BINARY_NAME)"; \
	if [ "$(GOOS)" = "windows" ]; then bin="$(BINARY_NAME).exe"; fi; \
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS) -X main.builtBy=make-release" \
		-o "$$stage/$$bin" $(MAIN_PATH); \
	if [ ! -s "$$stage/$$bin" ]; then \
		echo "错误: 二进制 $$stage/$$bin 不存在或为空，构建失败" >&2; \
		exit 1; \
	fi; \
	for f in README.md LICENSE CHANGELOG.md; do \
		if [ -f "$$f" ]; then cp "$$f" "$$stage/"; fi; \
	done; \
	archive_name="gitee-cli_$(VERSION_NO_V)_$(OS_NAME)_$(ARCH_NAME).$(EXT)"; \
	if [ "$(EXT)" = "zip" ]; then \
		if command -v zip >/dev/null 2>&1; then \
			(cd "$$stage" && zip -q -r "../$$archive_name" .); \
		else \
			(cd "$$stage" && python3 -m zipfile -c "../$$archive_name" $$(ls -A)); \
		fi; \
	else \
		tar -C "$$stage" -czf "$(DIST_DIR)/$$archive_name" .; \
	fi; \
	rm -rf "$$stage"; \
	echo "  -> $(DIST_DIR)/$$archive_name"

# 使用 GoReleaser 发布（需要 GITHUB_TOKEN）
.PHONY: release
release:
	@echo "$(BLUE)使用 GoReleaser 创建发布...$(NC)"
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "$(YELLOW)错误: goreleaser 未安装$(NC)"; \
		echo "安装方法: https://goreleaser.com/install/"; \
		exit 1; \
	fi
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		echo "$(YELLOW)错误: GITHUB_TOKEN 环境变量未设置$(NC)"; \
		exit 1; \
	fi
	goreleaser release --clean
	@echo "$(GREEN)发布完成$(NC)"

# 创建快照版本（本地测试，不发布）
.PHONY: release-snapshot
release-snapshot:
	@echo "$(BLUE)创建快照版本...$(NC)"
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "$(YELLOW)错误: goreleaser 未安装$(NC)"; \
		echo "安装方法: https://goreleaser.com/install/"; \
		exit 1; \
	fi
	goreleaser release --snapshot --clean --skip=publish
	@echo "$(GREEN)快照版本已创建$(NC)"
	@ls -lh $(DIST_DIR)

# 验证 GoReleaser 配置
.PHONY: release-check
release-check:
	@echo "$(BLUE)验证 GoReleaser 配置...$(NC)"
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "$(YELLOW)错误: goreleaser 未安装$(NC)"; \
		exit 1; \
	fi
	goreleaser check
	@echo "$(GREEN)配置验证通过$(NC)"
