package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// prViewOptions 收集 pr view 子命令的全部参数。
type prViewOptions struct {
	number   int64
	web      bool
	jsonOut  bool
	comments bool
	noColor  bool
}

// prViewData 聚合 pr view 输出所需的数据：PR 详情与（可选的）评论列表。
type prViewData struct {
	PR       *api.PullRequest `json:"pull_request"`
	Comments []api.Comment    `json:"comments,omitempty"`
}

// prViewEnv 聚合 pr view 的外部依赖，使核心流程可在测试中完全注入。
type prViewEnv struct {
	git          gitRunner
	loadConfig   func() (*config.Config, error)
	getPR        func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error)
	listComments func(ctx context.Context, host, token, owner, repo string, number int64) ([]api.Comment, error)
	openBrowser  func(url string) error
	out          io.Writer
	isTTY        func() bool
	now          func() time.Time
}

// defaultPRViewEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultPRViewEnv() prViewEnv {
	return prViewEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.GetPullRequest(ctx, owner, repo, number)
		},
		listComments: func(ctx context.Context, host, token, owner, repo string, number int64) ([]api.Comment, error) {
			client := api.NewClient(host, token)
			return client.ListPullRequestComments(ctx, owner, repo, number)
		},
		openBrowser: openBrowser,
		out:         os.Stdout,
		isTTY:       stdoutIsTTY,
		now:         time.Now,
	}
}

// newPRViewCmd 创建 pr view 子命令。
func newPRViewCmd() *cobra.Command {
	opts := prViewOptions{}

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "查看 Pull Request 详情",
		Long: `查看指定编号 Pull Request 的详细信息。

默认显示标题、状态、作者、分支与描述。使用 --comments 追加评论列表，
使用 --json 输出原始 JSON 便于脚本处理，使用 --web 在浏览器中打开。`,
		Example: `  # 查看 PR #123 详情
  gitee pr view 123

  # 查看 PR 并显示评论
  gitee pr view 123 --comments

  # 以 JSON 输出
  gitee pr view 123 --json

  # 在浏览器中打开
  gitee pr view 123 --web`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("无效的 PR 编号 %q，必须是正整数", args[0])
			}
			opts.number = n
			return runPRView(context.Background(), opts, defaultPRViewEnv())
		},
	}

	cmd.Flags().BoolVarP(&opts.web, "web", "w", false, "在浏览器中打开 PR")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVarP(&opts.comments, "comments", "c", false, "显示评论列表")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runPRView 执行 pr view 的核心流程。
func runPRView(ctx context.Context, opts prViewOptions, env prViewEnv) error {
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

	// 3. 获取 PR 详情
	pr, err := env.getPR(ctx, cfg.Host, cfg.Token, owner, repo, opts.number)
	if err != nil {
		return fmt.Errorf("获取 PR 详情失败: %w", err)
	}

	// 4. --web：直接打开浏览器并返回，不再拉取评论
	if opts.web {
		if err := env.openBrowser(pr.HTMLURL); err != nil {
			return fmt.Errorf("无法打开浏览器: %w", err)
		}
		fmt.Fprintf(env.out, "已在浏览器中打开 %s\n", pr.HTMLURL)
		return nil
	}

	// 5. 按需拉取评论
	var comments []api.Comment
	if opts.comments {
		comments, err = env.listComments(ctx, cfg.Host, cfg.Token, owner, repo, opts.number)
		if err != nil {
			return fmt.Errorf("获取 PR 评论失败: %w", err)
		}
	}

	// 6. JSON 输出
	if opts.jsonOut {
		return writeJSONValue(env.out, prViewData{PR: pr, Comments: comments})
	}

	// 7. 人类可读输出
	useColor := !opts.noColor && env.isTTY()
	return writePRView(env.out, pr, comments, opts.comments, env.now(), useColor)
}

// writePRView 以人类可读的格式输出 PR 详情，风格对齐 gh pr view。
func writePRView(w io.Writer, pr *api.PullRequest, comments []api.Comment, showComments bool, now time.Time, useColor bool) error {
	// 标题行 + 编号
	fmt.Fprintf(w, "%s %s\n", colorize(pr.Title, colorCyan, useColor), colorize(fmt.Sprintf("#%d", pr.Number), colorGray, useColor))

	// 元信息行：状态 · 作者 · 分支
	state := colorizeState(pr.State, useColor)
	author := pr.User.Login
	if author == "" {
		author = "未知"
	}
	fmt.Fprintf(w, "%s 由 %s 创建 · %s\n", state, author, relativeTime(pr.CreatedAt, now))
	fmt.Fprintf(w, "%s ← %s\n", pr.Base.Ref, pr.Head.Ref)

	// 合并状态提示
	switch {
	case pr.MergedAt != "":
		fmt.Fprintf(w, "%s\n", colorize("✔ 已合并", colorMagenta, useColor))
	case strings.EqualFold(pr.State, "closed"):
		fmt.Fprintf(w, "%s\n", colorize("✖ 已关闭", colorRed, useColor))
	case pr.Mergeable:
		fmt.Fprintf(w, "%s\n", colorize("✔ 可合并", colorGreen, useColor))
	default:
		fmt.Fprintf(w, "%s\n", colorize("⚠ 暂不可合并（可能存在冲突）", colorYellow, useColor))
	}

	// 描述
	fmt.Fprintln(w)
	if strings.TrimSpace(pr.Body) == "" {
		fmt.Fprintln(w, colorize("（无描述）", colorGray, useColor))
	} else {
		fmt.Fprintln(w, pr.Body)
	}

	// 链接
	fmt.Fprintf(w, "\n%s %s\n", colorize("查看链接:", colorGray, useColor), pr.HTMLURL)

	// 评论
	if showComments {
		fmt.Fprintln(w)
		writeComments(w, comments, now, useColor)
	}

	return nil
}

// writeComments 输出评论列表，PR 与 Issue 共用。
func writeComments(w io.Writer, comments []api.Comment, now time.Time, useColor bool) {
	if len(comments) == 0 {
		fmt.Fprintln(w, colorize("暂无评论。", colorGray, useColor))
		return
	}
	fmt.Fprintf(w, "%s\n", colorize(fmt.Sprintf("——— %d 条评论 ———", len(comments)), colorGray, useColor))
	for _, cm := range comments {
		author := cm.User.Login
		if author == "" {
			author = "未知"
		}
		fmt.Fprintf(w, "\n%s · %s\n", colorize(author, colorCyan, useColor), relativeTime(cm.CreatedAt, now))
		body := strings.TrimSpace(cm.Body)
		if body == "" {
			body = colorize("（空评论）", colorGray, useColor)
		}
		fmt.Fprintln(w, body)
	}
}
