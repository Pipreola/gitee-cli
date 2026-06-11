package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// ciStatusOptions 收集 ci status 子命令的全部参数。
type ciStatusOptions struct {
	ref     string
	branch  string
	jsonOut bool
	web     bool
	noColor bool
}

// ciStatusEnv 聚合 ci status 的外部依赖，使核心流程可在测试中完全注入。
type ciStatusEnv struct {
	git               gitRunner
	loadConfig        func() (*config.Config, error)
	getCombinedStatus func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error)
	listStatuses      func(ctx context.Context, host, token, owner, repo, ref string, input *api.ListCIStatusesInput) ([]api.CIStatus, error)
	openBrowser       func(url string) error
	out               io.Writer
	isTTY             func() bool
	now               func() time.Time
}

// defaultCIStatusEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultCIStatusEnv() ciStatusEnv {
	return ciStatusEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			client := api.NewClient(host, token)
			return client.GetCombinedStatus(ctx, owner, repo, ref)
		},
		listStatuses: func(ctx context.Context, host, token, owner, repo, ref string, input *api.ListCIStatusesInput) ([]api.CIStatus, error) {
			client := api.NewClient(host, token)
			return client.ListCIStatuses(ctx, owner, repo, ref, input)
		},
		openBrowser: openCIBrowser,
		out:         os.Stdout,
		isTTY:       stdoutIsTTY,
		now:         time.Now,
	}
}

// openCIBrowser 在默认浏览器中打开 URL（ci status 使用独立函数避免与 pr.go 中同名函数冲突）。
func openCIBrowser(url string) error {
	var cmd *exec.Cmd
	switch {
	case commandExists("xdg-open"):
		cmd = exec.Command("xdg-open", url)
	case commandExists("open"):
		cmd = exec.Command("open", url)
	case commandExists("start"):
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("无法检测到浏览器打开命令")
	}
	return cmd.Start()
}

// newCIStatusCmd 创建 ci status 子命令。
func newCIStatusCmd() *cobra.Command {
	opts := ciStatusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看仓库的 CI/CD 构建状态",
		Long: `查看仓库指定 ref（分支、tag 或 commit SHA）的 CI/CD 构建状态。

默认查看当前分支最新 commit 的聚合状态，包括所有 CI 上下文的结果。
使用 --branch 可指定分支，使用 --ref 可指定 commit SHA 或 tag。
使用 --json 输出原始 JSON 便于脚本处理。`,
		Example: `  # 查看默认分支（当前分支）的 CI 状态
  gitee ci status

  # 查看指定分支的 CI 状态
  gitee ci status --branch develop

  # 查看指定 commit 的 CI 状态
  gitee ci status --ref abc1234

  # 输出 JSON 格式
  gitee ci status --json

  # 在浏览器中打开 CI 页面
  gitee ci status --web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCIStatus(context.Background(), opts, defaultCIStatusEnv())
		},
	}

	cmd.Flags().StringVarP(&opts.branch, "branch", "b", "", "查看指定分支的 CI 状态（默认使用当前分支）")
	cmd.Flags().StringVar(&opts.ref, "ref", "", "查看指定 commit SHA 或 tag 的 CI 状态（优先级高于 --branch）")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVarP(&opts.web, "web", "w", false, "在浏览器中打开 CI 页面")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runCIStatus 执行 ci status 的核心流程。
func runCIStatus(ctx context.Context, opts ciStatusOptions, env ciStatusEnv) error {
	// 1. 加载配置，检查认证
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 2. 解析当前仓库
	owner, repo, err := getCurrentRepo(env.git)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	// 3. 确定查询的 ref：--ref > --branch > 当前分支
	ref := opts.ref
	if ref == "" {
		ref = opts.branch
	}
	if ref == "" {
		// 自动检测当前分支
		ref, err = getCurrentBranch(env.git)
		if err != nil {
			return fmt.Errorf("获取当前分支失败: %w", err)
		}
	}

	// 4. --web：构造 CI 页面 URL 并打开浏览器
	if opts.web {
		ciURL := buildCIPageURL(cfg.Host, owner, repo, ref)
		fmt.Fprintf(env.out, "正在浏览器中打开 %s ...\n", ciURL)
		return env.openBrowser(ciURL)
	}

	// 5. 查询聚合 CI 状态
	combined, err := env.getCombinedStatus(ctx, cfg.Host, cfg.Token, owner, repo, ref)
	if err != nil {
		// 若 API 返回 404，可能是该仓库未配置 CI，给出友好提示
		if isNotFoundError(err) {
			return fmt.Errorf("未找到 %s/%s@%s 的 CI 状态，请确认仓库已启用 CI/CD", owner, repo, ref)
		}
		return fmt.Errorf("查询 CI 状态失败: %w", err)
	}

	// 6. 输出
	if opts.jsonOut {
		return writeCIStatusJSON(env.out, combined)
	}
	useColor := !opts.noColor && env.isTTY()
	return writeCIStatus(env.out, combined, ref, owner, repo, env.now(), useColor)
}

// buildCIPageURL 构造 CI 页面 URL。
// 格式: https://gitee.com/:owner/:repo/commits/:ref
func buildCIPageURL(host, owner, repo, ref string) string {
	// host 可能是 "https://gitee.com/api/v5"，需要提取基础域名
	base := host
	if idx := strings.Index(base, "/api/"); idx != -1 {
		base = base[:idx]
	}
	base = strings.TrimRight(base, "/")
	return fmt.Sprintf("%s/%s/%s/commits/%s", base, owner, repo, ref)
}

// isNotFoundError 判断 err 是否为 Gitee API 的 404 错误。
func isNotFoundError(err error) bool {
	if apiErr, ok := err.(*api.APIError); ok {
		return apiErr.StatusCode == 404
	}
	return false
}

// writeCIStatusJSON 以 JSON 格式输出 CI 状态。
func writeCIStatusJSON(w io.Writer, combined *api.CombinedStatus) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(combined)
}

// writeCIStatus 以人类可读格式输出 CI 状态，风格对齐 gh/glab。
func writeCIStatus(w io.Writer, combined *api.CombinedStatus, ref, owner, repo string, now time.Time, useColor bool) error {
	// 标题行：聚合状态
	stateStr := formatCIState(combined.State, useColor)
	fmt.Fprintf(w, "%s/%s@%s  CI 聚合状态: %s\n", owner, repo, ref, stateStr)

	// 若无任何 CI 上下文
	if combined.TotalCount == 0 || len(combined.Statuses) == 0 {
		fmt.Fprintln(w, colorize("暂无 CI 状态记录，请确认仓库已配置 CI/CD。", colorGray, useColor))
		return nil
	}

	fmt.Fprintf(w, "共 %d 个 CI 上下文\n\n", combined.TotalCount)

	// 表格：各上下文状态
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tCONTEXT\tDESCRIPTION\tUPDATED")
	for _, s := range combined.Statuses {
		stateCell := formatCIState(s.State, useColor)
		context := s.Context
		if context == "" {
			context = colorize("-", colorGray, useColor)
		}
		desc := s.Description
		if desc == "" {
			desc = colorize("-", colorGray, useColor)
		}
		// 截断过长的描述
		if len([]rune(desc)) > 50 {
			desc = string([]rune(desc)[:49]) + "…"
		}
		updated := relativeTime(s.UpdatedAt, now)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", stateCell, context, desc, updated)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// 输出每条状态的 target_url（如果有）
	hasLinks := false
	for _, s := range combined.Statuses {
		if s.TargetURL != "" {
			if !hasLinks {
				fmt.Fprintln(w)
				hasLinks = true
			}
			fmt.Fprintf(w, "%s: %s\n", colorize(s.Context, colorCyan, useColor), s.TargetURL)
		}
	}

	return nil
}

// formatCIState 为 CI 状态着色并格式化。
// Gitee CI 状态值: pending / running / success / failed / error / canceled。
func formatCIState(state string, useColor bool) string {
	switch strings.ToLower(state) {
	case "success":
		return colorize("✔ success", colorGreen, useColor)
	case "failed", "failure":
		return colorize("✖ failed", colorRed, useColor)
	case "error":
		return colorize("✖ error", colorRed, useColor)
	case "pending":
		return colorize("● pending", colorYellow, useColor)
	case "running":
		return colorize("● running", colorYellow, useColor)
	case "canceled", "cancelled":
		return colorize("○ canceled", colorGray, useColor)
	default:
		if state == "" {
			return colorize("○ unknown", colorGray, useColor)
		}
		return colorize(state, colorGray, useColor)
	}
}
