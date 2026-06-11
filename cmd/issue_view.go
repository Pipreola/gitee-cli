package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// issueViewOptions 收集 issue view 子命令的全部参数。
type issueViewOptions struct {
	web      bool
	comments bool
	jsonOut  bool
	noColor  bool
}

// issueViewEnv 聚合 issue view 的外部依赖,使核心流程可在测试中完全注入。
type issueViewEnv struct {
	git            gitRunner
	loadConfig     func() (*config.Config, error)
	getIssue       func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error)
	listComments   func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error)
	openBrowser    func(url string) error
	out            io.Writer
	isTTY          func() bool
	now            func() time.Time
}

// defaultIssueViewEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultIssueViewEnv() issueViewEnv {
	return issueViewEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			client := api.NewClient(host, token)
			return client.GetIssue(ctx, owner, repo, number)
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			client := api.NewClient(host, token)
			return client.ListIssueComments(ctx, owner, repo, number)
		},
		openBrowser: openURL,
		out:         os.Stdout,
		isTTY:       stdoutIsTTY,
		now:         time.Now,
	}
}

// openURL 在默认浏览器中打开 URL，跨平台兼容。
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
	return cmd.Run()
}

// newIssueViewCmd 创建 issue view 子命令。
func newIssueViewCmd() *cobra.Command {
	opts := issueViewOptions{}

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "查看 Issue 详情",
		Long: `查看指定 Issue 的详细信息。

默认以人类可读的格式输出 Issue 标题、正文、状态、作者等信息。
使用 --comments 可以同时显示评论列表。
使用 --web 可以在浏览器中打开 Issue 页面。`,
		Example: `  # 查看 Issue 详情
  gitee issue view 42

  # 查看 Issue 并显示评论
  gitee issue view 42 --comments

  # 在浏览器中打开 Issue
  gitee issue view 42 --web

  # 输出 JSON 格式
  gitee issue view 42 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIssueView(context.Background(), args[0], opts, defaultIssueViewEnv())
		},
	}

	cmd.Flags().BoolVarP(&opts.web, "web", "w", false, "在浏览器中打开 Issue 页面")
	cmd.Flags().BoolVarP(&opts.comments, "comments", "c", false, "显示评论列表")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runIssueView 执行 issue view 的核心流程。
func runIssueView(ctx context.Context, number string, opts issueViewOptions, env issueViewEnv) error {
	// 1. 校验 Issue 编号（Gitee 支持字符串型编号）
	number = strings.TrimSpace(number)
	if number == "" {
		return fmt.Errorf("Issue 编号不能为空")
	}

	// 2. 加载配置，检查认证
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 3. 解析当前仓库
	owner, repo, err := getCurrentRepo(env.git)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	// 4. 调用 API 获取 Issue 详情
	issue, err := env.getIssue(ctx, cfg.Host, cfg.Token, owner, repo, number)
	if err != nil {
		return fmt.Errorf("查询 Issue 详情失败: %w", err)
	}

	// 5. 如果指定 --web，在浏览器中打开并退出
	if opts.web {
		if issue.HTMLURL == "" {
			return fmt.Errorf("Issue 无法在浏览器中打开：缺少 URL")
		}
		fmt.Fprintf(env.out, "正在浏览器中打开 %s ...\n", issue.HTMLURL)
		return env.openBrowser(issue.HTMLURL)
	}

	// 6. 如果指定 --comments，获取评论列表
	var comments []api.Comment
	if opts.comments {
		comments, err = env.listComments(ctx, cfg.Host, cfg.Token, owner, repo, number)
		if err != nil {
			return fmt.Errorf("查询评论列表失败: %w", err)
		}
	}

	// 7. 输出
	if opts.jsonOut {
		return writeIssueViewJSON(env.out, issue, comments)
	}
	useColor := !opts.noColor && env.isTTY()
	return writeIssueView(env.out, issue, comments, opts.comments, env.now(), useColor)
}

// writeIssueViewJSON 以 JSON 格式输出 Issue 详情。
func writeIssueViewJSON(w io.Writer, issue *api.Issue, comments []api.Comment) error {
	output := struct {
		Issue    *api.Issue     `json:"issue"`
		Comments []api.Comment  `json:"comments,omitempty"`
	}{
		Issue:    issue,
		Comments: comments,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(output)
}

// writeIssueView 以人类可读格式输出 Issue 详情，风格对齐 gh/glab。
func writeIssueView(w io.Writer, issue *api.Issue, comments []api.Comment, showComments bool, now time.Time, useColor bool) error {
	// 标题行
	fmt.Fprintf(w, "%s %s\n", colorize(issue.Number, colorCyan, useColor), issue.Title)

	// 状态栏
	state := colorizeIssueState(issue.State, useColor)
	created := relativeTime(issue.CreatedAt, now)
	fmt.Fprintf(w, "%s • %s opened %s\n", state, issue.User.Login, created)

	// 标签
	if len(issue.Labels) > 0 {
		labelNames := formatIssueLabels(issue.Labels)
		fmt.Fprintf(w, "标签: %s\n", colorize(labelNames, colorYellow, useColor))
	}

	// 指派人
	if issue.Assignee != nil {
		fmt.Fprintf(w, "指派给: %s\n", colorize(issue.Assignee.Login, colorCyan, useColor))
	}

	// 里程碑
	if issue.Milestone != nil {
		fmt.Fprintf(w, "里程碑: %s\n", colorize(issue.Milestone.Title, colorMagenta, useColor))
	}

	// 分隔线
	fmt.Fprintln(w, strings.Repeat("-", 80))

	// Issue 正文
	body := strings.TrimSpace(issue.Body)
	if body == "" {
		fmt.Fprintln(w, colorize("(无描述)", colorGray, useColor))
	} else {
		fmt.Fprintln(w, body)
	}

	// 元信息栏
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "评论数:\t%d\n", issue.Comments)
	fmt.Fprintf(tw, "创建时间:\t%s\n", issue.CreatedAt)
	fmt.Fprintf(tw, "更新时间:\t%s\n", issue.UpdatedAt)
	if issue.ClosedAt != "" {
		fmt.Fprintf(tw, "关闭时间:\t%s\n", issue.ClosedAt)
	}
	fmt.Fprintf(tw, "链接:\t%s\n", issue.HTMLURL)
	tw.Flush()

	// 评论列表（如果指定 --comments）
	if showComments && len(comments) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.Repeat("-", 80))
		fmt.Fprintf(w, "%s (%d)\n\n", colorize("评论", colorCyan, useColor), len(comments))

		for i, comment := range comments {
			commentCreated := relativeTime(comment.CreatedAt, now)
			fmt.Fprintf(w, "%s commented %s:\n", colorize(comment.User.Login, colorCyan, useColor), commentCreated)
			fmt.Fprintln(w, strings.TrimSpace(comment.Body))
			if i < len(comments)-1 {
				fmt.Fprintln(w)
			}
		}
	}

	return nil
}
