package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// writeJSONValue 以缩进 JSON 输出任意值，禁用 HTML 转义，供各 view 命令复用。
func writeJSONValue(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// repoViewOptions 收集 repo view 子命令的全部参数。
type repoViewOptions struct {
	// repo 是可选的 [owner/]repo 定位参数；为空时从当前 git remote 推断。
	repo    string
	web     bool
	jsonOut bool
	noColor bool
}

// repoViewEnv 聚合 repo view 的外部依赖，使核心流程可在测试中完全注入。
type repoViewEnv struct {
	git         gitRunner
	loadConfig  func() (*config.Config, error)
	getRepo     func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error)
	openBrowser func(url string) error
	out         io.Writer
	isTTY       func() bool
	now         func() time.Time
}

// defaultRepoViewEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultRepoViewEnv() repoViewEnv {
	return repoViewEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			client := api.NewClient(host, token)
			return client.GetRepository(ctx, owner, repo)
		},
		openBrowser: openBrowser,
		out:         os.Stdout,
		isTTY:       stdoutIsTTY,
		now:         time.Now,
	}
}

// newRepoCmd 创建 repo 命令。
func newRepoCmd() *cobra.Command {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "管理与查看仓库",
		Long:  "查看 Gitee 仓库信息。",
	}
	repoCmd.AddCommand(newRepoViewCmd())
	repoCmd.AddCommand(newRepoCloneCmd())
	return repoCmd
}

// newRepoViewCmd 创建 repo view 子命令。
func newRepoViewCmd() *cobra.Command {
	opts := repoViewOptions{}

	cmd := &cobra.Command{
		Use:   "view [owner/repo]",
		Short: "查看仓库信息",
		Long: `查看仓库的基本信息与统计数据。

不指定参数时从当前目录的 git remote 推断仓库；也可显式传入 owner/repo 定位任意仓库。
使用 --json 输出原始 JSON，使用 --web 在浏览器中打开仓库主页。`,
		Example: `  # 查看当前仓库
  gitee repo view

  # 查看指定仓库
  gitee repo view owner/repo

  # 以 JSON 输出
  gitee repo view --json

  # 在浏览器中打开
  gitee repo view --web`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.repo = args[0]
			}
			return runRepoView(context.Background(), opts, defaultRepoViewEnv())
		},
	}

	cmd.Flags().BoolVarP(&opts.web, "web", "w", false, "在浏览器中打开仓库")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runRepoView 执行 repo view 的核心流程。
func runRepoView(ctx context.Context, opts repoViewOptions, env repoViewEnv) error {
	// 1. 加载配置，检查认证
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 2. 确定 owner/repo：优先使用显式参数，否则从 git remote 推断
	var owner, repo string
	if opts.repo != "" {
		owner, repo, err = parseRepoArg(opts.repo)
		if err != nil {
			return err
		}
	} else {
		owner, repo, err = getCurrentRepo(env.git)
		if err != nil {
			return fmt.Errorf("获取仓库信息失败: %w", err)
		}
	}

	// 3. 获取仓库详情
	r, err := env.getRepo(ctx, cfg.Host, cfg.Token, owner, repo)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	// 4. --web：打开浏览器
	if opts.web {
		if err := env.openBrowser(r.HTMLURL); err != nil {
			return fmt.Errorf("无法打开浏览器: %w", err)
		}
		fmt.Fprintf(env.out, "已在浏览器中打开 %s\n", r.HTMLURL)
		return nil
	}

	// 5. JSON 输出
	if opts.jsonOut {
		return writeJSONValue(env.out, r)
	}

	// 6. 人类可读输出
	useColor := !opts.noColor && env.isTTY()
	return writeRepoView(env.out, r, env.now(), useColor)
}

// parseRepoArg 解析 owner/repo 形式的定位参数。
func parseRepoArg(s string) (owner, repo string, err error) {
	parts := splitOwnerRepo(s)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("无效的仓库定位 %q，期望格式 owner/repo", s)
	}
	return parts[0], parts[1], nil
}

// splitOwnerRepo 将 "owner/repo" 拆成两段（去掉可能的 .git 后缀）。
func splitOwnerRepo(s string) []string {
	s = trimGitSuffix(s)
	idx := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

// trimGitSuffix 去掉字符串末尾的 ".git" 后缀。
func trimGitSuffix(s string) string {
	const suffix = ".git"
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// writeRepoView 以人类可读格式输出仓库信息，风格对齐 gh repo view。
func writeRepoView(w io.Writer, r *api.Repository, now time.Time, useColor bool) error {
	// 标题：full_name（可见性）
	visibility := "public"
	visColor := colorGreen
	if r.Private {
		visibility = "private"
		visColor = colorYellow
	}
	name := r.FullName
	if name == "" {
		name = r.Name
	}
	fmt.Fprintf(w, "%s %s\n", colorize(name, colorCyan, useColor), colorize("("+visibility+")", visColor, useColor))

	// 描述
	if r.Description != "" {
		fmt.Fprintf(w, "%s\n", r.Description)
	} else {
		fmt.Fprintln(w, colorize("（无描述）", colorGray, useColor))
	}
	fmt.Fprintln(w)

	// 统计信息（对齐表格）
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if r.Language != "" {
		fmt.Fprintf(tw, "语言\t%s\n", r.Language)
	}
	fmt.Fprintf(tw, "Star\t%d\n", r.StargazersCount)
	fmt.Fprintf(tw, "Fork\t%d\n", r.ForksCount)
	fmt.Fprintf(tw, "Watch\t%d\n", r.WatchersCount)
	fmt.Fprintf(tw, "开放 Issue\t%d\n", r.OpenIssuesCount)
	if r.DefaultBranch != "" {
		fmt.Fprintf(tw, "默认分支\t%s\n", r.DefaultBranch)
	}
	if r.Homepage != "" {
		fmt.Fprintf(tw, "主页\t%s\n", r.Homepage)
	}
	if r.PushedAt != "" {
		fmt.Fprintf(tw, "最近活动\t%s\n", relativeTime(r.PushedAt, now))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// 链接
	fmt.Fprintf(w, "\n%s %s\n", colorize("查看链接:", colorGray, useColor), r.HTMLURL)
	return nil
}
