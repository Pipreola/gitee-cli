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

// 终端 ANSI 颜色码，用于状态着色（与 gh/glab 风格一致）。
const (
	colorReset   = "\033[0m"
	colorGreen   = "\033[32m"
	colorMagenta = "\033[35m"
	colorRed     = "\033[31m"
	colorYellow  = "\033[33m"
	colorGray    = "\033[90m"
	colorCyan    = "\033[36m"
)

// prListOptions 收集 pr list 子命令的全部参数。
type prListOptions struct {
	state     string
	author    string
	labels    string
	base      string
	head      string
	direction string
	sort      string
	limit     int
	jsonOut   bool
	verbose   bool
	noColor   bool
}

// prListEnv 聚合 pr list 的外部依赖，使核心流程可在测试中完全注入。
type prListEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	listPRs    func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error)
	out        io.Writer
	// isTTY 标识 out 是否为终端，用于决定默认是否启用颜色。
	isTTY func() bool
	// now 用于格式化相对时间，便于测试注入。
	now func() time.Time
}

// defaultPRListEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultPRListEnv() prListEnv {
	return prListEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.ListPullRequests(ctx, owner, repo, input)
		},
		out:   os.Stdout,
		isTTY: stdoutIsTTY,
		now:   time.Now,
	}
}

// stdoutIsTTY 检查标准输出是否连接到终端。
func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// newPRListCmd 创建 pr list 子命令。
func newPRListCmd() *cobra.Command {
	opts := prListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出 Pull Request",
		Long: `列出当前仓库的 Pull Request 列表。

支持按状态、作者、标签、分支等条件筛选，输出格式与 gh/glab 一致。
默认仅显示开放（open）状态的 PR。使用 --json 输出原始 JSON 便于脚本处理。`,
		Example: `  # 列出当前仓库所有开放 PR
  gitee pr list

  # 列出已合并的 PR
  gitee pr list --state merged

  # 列出特定作者的 PR
  gitee pr list --author alice

  # 列出带特定标签的 PR
  gitee pr list --label bug,urgent

  # 限制数量并输出 JSON
  gitee pr list --limit 50 --json

  # 按更新时间倒序
  gitee pr list --sort updated --direction desc

  # 详细模式（显示更多列）
  gitee pr list --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPRList(context.Background(), opts, defaultPRListEnv())
		},
	}

	cmd.Flags().StringVarP(&opts.state, "state", "s", "open", "PR 状态过滤：open/closed/merged/all")
	cmd.Flags().StringVarP(&opts.author, "author", "A", "", "按作者用户名过滤（@me 表示当前登录用户）")
	cmd.Flags().StringVarP(&opts.labels, "label", "l", "", "按标签过滤，逗号分隔")
	cmd.Flags().StringVar(&opts.base, "base", "", "按目标分支过滤")
	cmd.Flags().StringVar(&opts.head, "head", "", "按源分支过滤（格式 namespace:branch）")
	cmd.Flags().StringVar(&opts.direction, "direction", "desc", "排序方向：asc/desc")
	cmd.Flags().StringVar(&opts.sort, "sort", "created", "排序字段：created/updated/popularity/long-running")
	cmd.Flags().IntVarP(&opts.limit, "limit", "L", 30, "返回的最大 PR 数量（1-100）")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "详细模式：显示更多列（作者、分支、更新时间）")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runPRList 执行 pr list 的核心流程。
func runPRList(ctx context.Context, opts prListOptions, env prListEnv) error {
	// 1. 校验参数
	if err := validatePRListOptions(&opts); err != nil {
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

	// 4. 处理 @me：替换为已登录用户名（不触发额外网络请求，使用配置中的 user 字段）
	authorFilter := opts.author
	if authorFilter == "@me" {
		if cfg.User == "" {
			return fmt.Errorf("无法解析 @me：本地配置未保存登录用户名，请重新执行 'gitee auth login'")
		}
		authorFilter = cfg.User
	}

	// 5. 构造 API 请求。Gitee v5 接受 open/closed/merged/all。
	input := &api.ListPullRequestsInput{
		State:     opts.state,
		Head:      opts.head,
		Base:      opts.base,
		Sort:      opts.sort,
		Direction: opts.direction,
		Labels:    opts.labels,
		PerPage:   opts.limit,
		Page:      1,
	}

	// 6. 调用 API
	prs, err := env.listPRs(ctx, cfg.Host, cfg.Token, owner, repo, input)
	if err != nil {
		return fmt.Errorf("查询 PR 列表失败: %w", err)
	}

	// 7. 客户端按 author 过滤（Gitee 接口不支持该参数）
	if authorFilter != "" {
		prs = filterByAuthor(prs, authorFilter)
	}

	// 8. 限制数量（API 也按 per_page 限了，这里再兜底一次以应对客户端过滤后的边界）
	if opts.limit > 0 && len(prs) > opts.limit {
		prs = prs[:opts.limit]
	}

	// 9. 输出
	if opts.jsonOut {
		return writeJSON(env.out, prs)
	}
	useColor := !opts.noColor && env.isTTY()
	return writeTable(env.out, prs, owner, repo, opts, env.now(), useColor)
}

// validatePRListOptions 校验 pr list 的输入参数。
func validatePRListOptions(opts *prListOptions) error {
	switch opts.state {
	case "open", "closed", "merged", "all":
	default:
		return fmt.Errorf("无效的 --state 值 %q，仅支持 open/closed/merged/all", opts.state)
	}
	switch opts.direction {
	case "asc", "desc":
	default:
		return fmt.Errorf("无效的 --direction 值 %q，仅支持 asc/desc", opts.direction)
	}
	switch opts.sort {
	case "created", "updated", "popularity", "long-running":
	default:
		return fmt.Errorf("无效的 --sort 值 %q，仅支持 created/updated/popularity/long-running", opts.sort)
	}
	if opts.limit < 1 || opts.limit > 100 {
		return fmt.Errorf("无效的 --limit 值 %d，须在 1-100 之间", opts.limit)
	}
	return nil
}

// filterByAuthor 在客户端按作者用户名过滤 PR 列表。
// 大小写不敏感，匹配 PullRequest.User.Login。
func filterByAuthor(prs []api.PullRequest, author string) []api.PullRequest {
	want := strings.ToLower(strings.TrimSpace(author))
	if want == "" {
		return prs
	}
	out := make([]api.PullRequest, 0, len(prs))
	for _, pr := range prs {
		if strings.EqualFold(pr.User.Login, want) {
			out = append(out, pr)
		}
	}
	return out
}

// writeJSON 以 JSON 格式输出 PR 列表（始终包含原始字段）。
func writeJSON(w io.Writer, prs []api.PullRequest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(prs)
}

// writeTable 以表格形式输出 PR 列表，风格对齐 gh/glab。
// 注：API 已按 sort/direction 排过，这里不再做二次排序。
func writeTable(w io.Writer, prs []api.PullRequest, owner, repo string, opts prListOptions, now time.Time, useColor bool) error {
	if len(prs) == 0 {
		_, err := fmt.Fprintf(w, "%s/%s 中没有匹配的 Pull Request。\n", owner, repo)
		return err
	}

	fmt.Fprintf(w, "Showing %d %s in %s/%s\n\n", len(prs), pluralize(len(prs), "pull request", "pull requests"), owner, repo)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if opts.verbose {
		fmt.Fprintln(tw, "ID\tTITLE\tSTATE\tBRANCHES\tAUTHOR\tCREATED\tUPDATED")
	} else {
		fmt.Fprintln(tw, "ID\tTITLE\tSTATE\tBRANCHES\tCREATED")
	}

	for _, pr := range prs {
		idCell := colorize(fmt.Sprintf("#%d", pr.Number), colorCyan, useColor)
		title := truncate(pr.Title, 60)
		state := colorizeState(pr.State, useColor)
		branches := fmt.Sprintf("%s ← %s", pr.Base.Ref, pr.Head.Ref)
		created := relativeTime(pr.CreatedAt, now)

		if opts.verbose {
			updated := relativeTime(pr.UpdatedAt, now)
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", idCell, title, state, branches, pr.User.Login, created, updated)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", idCell, title, state, branches, created)
		}
	}
	return tw.Flush()
}

// truncate 截断字符串到最大长度，超出部分用省略号代替（按 rune 计长度）。
// 当字符串本身不超长时原样返回；当 max <= 1 且需要截断时返回单字符省略号。
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// colorize 仅在 useColor 为 true 时包裹 ANSI 颜色码。
func colorize(s, color string, useColor bool) string {
	if !useColor || color == "" {
		return s
	}
	return color + s + colorReset
}

// colorizeState 为 PR 状态着色：open=绿，merged=紫，closed=红，其它=灰。
func colorizeState(state string, useColor bool) string {
	switch strings.ToLower(state) {
	case "open":
		return colorize("OPEN", colorGreen, useColor)
	case "merged":
		return colorize("MERGED", colorMagenta, useColor)
	case "closed":
		return colorize("CLOSED", colorRed, useColor)
	case "progressing":
		return colorize("PROGRESS", colorYellow, useColor)
	default:
		return colorize(strings.ToUpper(state), colorGray, useColor)
	}
}

// relativeTime 将 RFC3339 时间转换为人类可读的相对时间（"2 小时前"、"3 天前"等）。
// 解析失败时返回原字符串。
func relativeTime(s string, now time.Time) string {
	if s == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// 尝试 Gitee 偶尔返回的 "+08:00" 格式
		t, err = time.Parse("2006-01-02T15:04:05-07:00", s)
		if err != nil {
			return s
		}
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d 个月前", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%d 年前", int(d.Hours()/24/365))
	}
}

// pluralize 根据数量返回单/复数形式。
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
