package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// issueListOptions 收集 issue list 子命令的全部参数。
type issueListOptions struct {
	state     string
	author    string
	assignee  string
	labels    string
	direction string
	sort      string
	limit     int
	jsonOut   bool
	verbose   bool
	noColor   bool
}

// issueListEnv 聚合 issue list 的外部依赖,使核心流程可在测试中完全注入。
type issueListEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	listIssues func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error)
	out        io.Writer
	// isTTY 标识 out 是否为终端,用于决定默认是否启用颜色。
	isTTY func() bool
	// now 用于格式化相对时间,便于测试注入。
	now func() time.Time
}

// defaultIssueListEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultIssueListEnv() issueListEnv {
	return issueListEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
			client := api.NewClient(host, token)
			return client.ListIssues(ctx, owner, repo, input)
		},
		out:   os.Stdout,
		isTTY: stdoutIsTTY,
		now:   time.Now,
	}
}

// newIssueListCmd 创建 issue list 子命令。
func newIssueListCmd() *cobra.Command {
	opts := issueListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出 Issue",
		Long: `列出当前仓库的 Issue 列表。

支持按状态、作者、标签等条件筛选,输出格式与 gh/glab 一致。
默认仅显示开放（open）状态的 Issue。使用 --json 输出原始 JSON 便于脚本处理。`,
		Example: `  # 列出当前仓库所有开放 Issue
  gitee issue list

  # 列出已关闭的 Issue
  gitee issue list --state closed

  # 列出特定作者的 Issue
  gitee issue list --author alice

  # 列出指派给我的 Issue
  gitee issue list --assignee @me

  # 列出带特定标签的 Issue
  gitee issue list --label bug,urgent

  # 限制数量并输出 JSON
  gitee issue list --limit 50 --json

  # 按更新时间倒序
  gitee issue list --sort updated --direction desc

  # 详细模式（显示更多列）
  gitee issue list --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIssueList(context.Background(), opts, defaultIssueListEnv())
		},
	}

	cmd.Flags().StringVarP(&opts.state, "state", "s", "open", "Issue 状态过滤：open/closed/progressing/rejected/all")
	cmd.Flags().StringVarP(&opts.author, "author", "A", "", "按作者用户名过滤（@me 表示当前登录用户）")
	cmd.Flags().StringVar(&opts.assignee, "assignee", "", "按指派人过滤（@me 表示当前登录用户）")
	cmd.Flags().StringVarP(&opts.labels, "label", "l", "", "按标签过滤，逗号分隔")
	cmd.Flags().StringVar(&opts.direction, "direction", "desc", "排序方向：asc/desc")
	cmd.Flags().StringVar(&opts.sort, "sort", "created", "排序字段：created/updated/comments")
	cmd.Flags().IntVarP(&opts.limit, "limit", "L", 30, "返回的最大 Issue 数量（1-100）")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "详细模式：显示更多列（指派人、标签、更新时间）")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runIssueList 执行 issue list 的核心流程。
func runIssueList(ctx context.Context, opts issueListOptions, env issueListEnv) error {
	// 1. 校验参数
	if err := validateIssueListOptions(&opts); err != nil {
		return err
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

	// 4. 处理 @me：替换为已登录用户名
	authorFilter := opts.author
	if authorFilter == "@me" {
		if cfg.User == "" {
			return fmt.Errorf("无法解析 @me：本地配置未保存登录用户名，请重新执行 'gitee auth login'")
		}
		authorFilter = cfg.User
	}

	assigneeFilter := opts.assignee
	if assigneeFilter == "@me" {
		if cfg.User == "" {
			return fmt.Errorf("无法解析 @me：本地配置未保存登录用户名，请重新执行 'gitee auth login'")
		}
		assigneeFilter = cfg.User
	}

	// 5. 构造 API 请求。Gitee v5 接受 open/closed/progressing/rejected/all。
	input := &api.ListIssuesInput{
		State:     opts.state,
		Sort:      opts.sort,
		Direction: opts.direction,
		Labels:    opts.labels,
		PerPage:   opts.limit,
		Page:      1,
	}

	// 6. 调用 API
	issues, err := env.listIssues(ctx, cfg.Host, cfg.Token, owner, repo, input)
	if err != nil {
		return fmt.Errorf("查询 Issue 列表失败: %w", err)
	}

	// 7. 客户端按 author/assignee 过滤（Gitee 接口不支持这些参数）
	if authorFilter != "" {
		issues = filterIssuesByAuthor(issues, authorFilter)
	}
	if assigneeFilter != "" {
		issues = filterIssuesByAssignee(issues, assigneeFilter)
	}

	// 8. 限制数量
	if opts.limit > 0 && len(issues) > opts.limit {
		issues = issues[:opts.limit]
	}

	// 9. 输出
	if opts.jsonOut {
		return writeIssueListJSON(env.out, issues)
	}
	useColor := !opts.noColor && env.isTTY()
	return writeIssueListTable(env.out, issues, owner, repo, opts, env.now(), useColor)
}

// validateIssueListOptions 校验 issue list 的输入参数。
func validateIssueListOptions(opts *issueListOptions) error {
	switch opts.state {
	case "open", "closed", "progressing", "rejected", "all":
	default:
		return fmt.Errorf("无效的 --state 值 %q，仅支持 open/closed/progressing/rejected/all", opts.state)
	}
	switch opts.direction {
	case "asc", "desc":
	default:
		return fmt.Errorf("无效的 --direction 值 %q，仅支持 asc/desc", opts.direction)
	}
	switch opts.sort {
	case "created", "updated", "comments":
	default:
		return fmt.Errorf("无效的 --sort 值 %q，仅支持 created/updated/comments", opts.sort)
	}
	if opts.limit < 1 || opts.limit > 100 {
		return fmt.Errorf("无效的 --limit 值 %d，须在 1-100 之间", opts.limit)
	}
	return nil
}

// filterIssuesByAuthor 在客户端按作者用户名过滤 Issue 列表。
func filterIssuesByAuthor(issues []api.Issue, author string) []api.Issue {
	want := strings.ToLower(strings.TrimSpace(author))
	if want == "" {
		return issues
	}
	out := make([]api.Issue, 0, len(issues))
	for _, issue := range issues {
		if strings.EqualFold(issue.User.Login, want) {
			out = append(out, issue)
		}
	}
	return out
}

// filterIssuesByAssignee 在客户端按指派人用户名过滤 Issue 列表。
func filterIssuesByAssignee(issues []api.Issue, assignee string) []api.Issue {
	want := strings.ToLower(strings.TrimSpace(assignee))
	if want == "" {
		return issues
	}
	out := make([]api.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Assignee != nil && strings.EqualFold(issue.Assignee.Login, want) {
			out = append(out, issue)
		}
	}
	return out
}

// writeIssueListJSON 以 JSON 格式输出 Issue 列表。
func writeIssueListJSON(w io.Writer, issues []api.Issue) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(issues)
}

// writeIssueListTable 以表格形式输出 Issue 列表，风格对齐 gh/glab。
func writeIssueListTable(w io.Writer, issues []api.Issue, owner, repo string, opts issueListOptions, now time.Time, useColor bool) error {
	if len(issues) == 0 {
		_, err := fmt.Fprintf(w, "%s/%s 中没有匹配的 Issue。\n", owner, repo)
		return err
	}

	fmt.Fprintf(w, "Showing %d %s in %s/%s\n\n", len(issues), pluralize(len(issues), "issue", "issues"), owner, repo)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if opts.verbose {
		fmt.Fprintln(tw, "NUMBER\tTITLE\tSTATE\tASSIGNEE\tLABELS\tCREATED\tUPDATED")
	} else {
		fmt.Fprintln(tw, "NUMBER\tTITLE\tSTATE\tAUTHOR\tCREATED")
	}

	for _, issue := range issues {
		numberCell := colorize(issue.Number, colorCyan, useColor)
		title := truncate(issue.Title, 60)
		state := colorizeIssueState(issue.State, useColor)
		created := relativeTime(issue.CreatedAt, now)

		if opts.verbose {
			assignee := "-"
			if issue.Assignee != nil {
				assignee = issue.Assignee.Login
			}
			labels := formatIssueLabels(issue.Labels)
			updated := relativeTime(issue.UpdatedAt, now)
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", numberCell, title, state, assignee, labels, created, updated)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", numberCell, title, state, issue.User.Login, created)
		}
	}
	return tw.Flush()
}

// colorizeIssueState 为 Issue 状态着色：open=绿，progressing=黄，closed=红，rejected=灰。
func colorizeIssueState(state string, useColor bool) string {
	switch strings.ToLower(state) {
	case "open":
		return colorize("OPEN", colorGreen, useColor)
	case "progressing":
		return colorize("PROGRESS", colorYellow, useColor)
	case "closed":
		return colorize("CLOSED", colorRed, useColor)
	case "rejected":
		return colorize("REJECTED", colorGray, useColor)
	default:
		return colorize(strings.ToUpper(state), colorGray, useColor)
	}
}

// formatIssueLabels 格式化 Issue 标签列表，逗号分隔。
func formatIssueLabels(labels []struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}) string {
	if len(labels) == 0 {
		return "-"
	}
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return strings.Join(names, ",")
}
