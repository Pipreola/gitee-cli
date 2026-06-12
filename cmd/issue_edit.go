package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// newIssueEditCmd 创建 issue edit 子命令。
func newIssueEditCmd() *cobra.Command {
	var (
		title     string
		body      string
		labels    string
		assignee  string
		milestone int64
	)

	cmd := &cobra.Command{
		Use:   "edit <number>",
		Short: "编辑 Issue",
		Long: `编辑指定编号的 Issue 的标题、正文、标签、指派人或里程碑。

仅修改显式指定的字段（未指定的 flag 保持不变）。
至少需要指定一个待修改字段。`,
		Example: `  # 修改标题
  gitee issue edit I123 --title "新标题"

  # 同时修改正文与标签
  gitee issue edit I123 --body "更新后的描述" --labels bug,urgent

  # 重新指派
  gitee issue edit I123 --assignee user1

  # 清空标签（传空字符串）
  gitee issue edit I123 --labels ""`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number := strings.TrimSpace(args[0])
			if number == "" {
				return fmt.Errorf("Issue 编号不能为空")
			}
			opts := issueEditOptions{
				number:           number,
				title:            title,
				titleChanged:     cmd.Flags().Changed("title"),
				body:             body,
				bodyChanged:      cmd.Flags().Changed("body"),
				labels:           labels,
				labelsChanged:    cmd.Flags().Changed("labels"),
				assignee:         assignee,
				assigneeChanged:  cmd.Flags().Changed("assignee"),
				milestone:        milestone,
				milestoneChanged: cmd.Flags().Changed("milestone"),
			}
			return runIssueEdit(context.Background(), opts, defaultIssueEditEnv())
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "新的 Issue 标题")
	cmd.Flags().StringVarP(&body, "body", "b", "", "新的 Issue 描述")
	cmd.Flags().StringVarP(&labels, "labels", "l", "", "标签，逗号分隔（传空字符串可清空）")
	cmd.Flags().StringVarP(&assignee, "assignee", "a", "", "指派人登录名（传空字符串可清空）")
	cmd.Flags().Int64VarP(&milestone, "milestone", "m", 0, "里程碑编号")

	return cmd
}

// issueEditOptions 收集 issue edit 子命令的参数。
// 每个字段附带一个 *Changed 标志，用于区分「未指定」与「显式设为空值」。
type issueEditOptions struct {
	number           string
	title            string
	titleChanged     bool
	body             string
	bodyChanged      bool
	labels           string
	labelsChanged    bool
	assignee         string
	assigneeChanged  bool
	milestone        int64
	milestoneChanged bool
}

// issueEditEnv 聚合 issue edit 的外部依赖。
type issueEditEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	editIssue  func(ctx context.Context, host, token, owner, repo, number string, input *api.EditIssueInput) (*api.Issue, error)
	out        io.Writer
}

// defaultIssueEditEnv 返回基于真实依赖的环境。
func defaultIssueEditEnv() issueEditEnv {
	return issueEditEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		editIssue: func(ctx context.Context, host, token, owner, repo, number string, input *api.EditIssueInput) (*api.Issue, error) {
			client := api.NewClient(host, token)
			return client.EditIssue(ctx, owner, repo, number, input)
		},
		out: os.Stdout,
	}
}

// runIssueEdit 执行 issue edit 的核心流程。
func runIssueEdit(ctx context.Context, opts issueEditOptions, env issueEditEnv) error {
	// 1. 至少指定一个待修改字段
	if !opts.titleChanged && !opts.bodyChanged && !opts.labelsChanged &&
		!opts.assigneeChanged && !opts.milestoneChanged {
		return fmt.Errorf("至少需要指定一个待修改字段（--title/--body/--labels/--assignee/--milestone）")
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

	// 4. 仅装配显式指定的字段
	input := &api.EditIssueInput{}
	if opts.titleChanged {
		input.Title = &opts.title
	}
	if opts.bodyChanged {
		input.Body = &opts.body
	}
	if opts.labelsChanged {
		input.Labels = &opts.labels
	}
	if opts.assigneeChanged {
		input.Assignee = &opts.assignee
	}
	if opts.milestoneChanged {
		input.MilestoneNumber = &opts.milestone
	}

	// 5. 调用 API
	issue, err := env.editIssue(ctx, cfg.Host, cfg.Token, owner, repo, opts.number, input)
	if err != nil {
		return fmt.Errorf("编辑 Issue 失败: %w", err)
	}

	// 6. 输出成功信息
	fmt.Fprintf(env.out, "✅ Issue %s 已更新\n", issue.Number)
	fmt.Fprintf(env.out, "   标题: %s\n", issue.Title)
	fmt.Fprintf(env.out, "   链接: %s\n", issue.HTMLURL)

	return nil
}
