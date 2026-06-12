package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// newPREditCmd 创建 pr edit 子命令。
func newPREditCmd() *cobra.Command {
	var (
		title     string
		body      string
		labels    string
		assignees string
		milestone int64
	)

	cmd := &cobra.Command{
		Use:   "edit <number>",
		Short: "编辑 Pull Request",
		Long: `编辑指定编号的 Pull Request 的标题、正文、标签、审阅者或里程碑。

仅修改显式指定的字段（未指定的 flag 保持不变）。
至少需要指定一个待修改字段。`,
		Example: `  # 修改标题
  gitee pr edit 123 --title "新标题"

  # 同时修改正文与标签
  gitee pr edit 123 --body "更新后的描述" --labels bug,urgent

  # 重新指派审阅者
  gitee pr edit 123 --assignees user1,user2

  # 清空标签（传空字符串）
  gitee pr edit 123 --labels ""`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || number <= 0 {
				return fmt.Errorf("无效的 PR 编号 %q，必须是正整数", args[0])
			}
			opts := prEditOptions{
				number:           number,
				title:            title,
				titleChanged:     cmd.Flags().Changed("title"),
				body:             body,
				bodyChanged:      cmd.Flags().Changed("body"),
				labels:           labels,
				labelsChanged:    cmd.Flags().Changed("labels"),
				assignees:        assignees,
				assigneesChanged: cmd.Flags().Changed("assignees"),
				milestone:        milestone,
				milestoneChanged: cmd.Flags().Changed("milestone"),
			}
			return runPREdit(context.Background(), opts, defaultPREditEnv())
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "新的 PR 标题")
	cmd.Flags().StringVarP(&body, "body", "b", "", "新的 PR 描述")
	cmd.Flags().StringVarP(&labels, "labels", "l", "", "标签，逗号分隔（传空字符串可清空）")
	cmd.Flags().StringVarP(&assignees, "assignees", "a", "", "审阅者，逗号分隔的用户名（传空字符串可清空）")
	cmd.Flags().Int64VarP(&milestone, "milestone", "m", 0, "里程碑编号")

	return cmd
}

// prEditOptions 收集 pr edit 子命令的参数。
// 每个字段附带一个 *Changed 标志，用于区分「未指定」与「显式设为空值」。
type prEditOptions struct {
	number           int64
	title            string
	titleChanged     bool
	body             string
	bodyChanged      bool
	labels           string
	labelsChanged    bool
	assignees        string
	assigneesChanged bool
	milestone        int64
	milestoneChanged bool
}

// prEditEnv 聚合 pr edit 的外部依赖。
type prEditEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	editPR     func(ctx context.Context, host, token, owner, repo string, number int64, input *api.EditPullRequestInput) (*api.PullRequest, error)
	out        io.Writer
}

// defaultPREditEnv 返回基于真实依赖的环境。
func defaultPREditEnv() prEditEnv {
	return prEditEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		editPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.EditPullRequestInput) (*api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.EditPullRequest(ctx, owner, repo, number, input)
		},
		out: os.Stdout,
	}
}

// runPREdit 执行 pr edit 的核心流程。
func runPREdit(ctx context.Context, opts prEditOptions, env prEditEnv) error {
	// 1. 至少指定一个待修改字段
	if !opts.titleChanged && !opts.bodyChanged && !opts.labelsChanged &&
		!opts.assigneesChanged && !opts.milestoneChanged {
		return fmt.Errorf("至少需要指定一个待修改字段（--title/--body/--labels/--assignees/--milestone）")
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
	input := &api.EditPullRequestInput{}
	if opts.titleChanged {
		input.Title = &opts.title
	}
	if opts.bodyChanged {
		input.Body = &opts.body
	}
	if opts.labelsChanged {
		input.Labels = &opts.labels
	}
	if opts.assigneesChanged {
		input.Assignees = &opts.assignees
	}
	if opts.milestoneChanged {
		input.MilestoneNumber = &opts.milestone
	}

	// 5. 调用 API
	pr, err := env.editPR(ctx, cfg.Host, cfg.Token, owner, repo, opts.number, input)
	if err != nil {
		return fmt.Errorf("编辑 PR 失败: %w", err)
	}

	// 6. 输出成功信息
	fmt.Fprintf(env.out, "✅ PR #%d 已更新\n", pr.Number)
	fmt.Fprintf(env.out, "   标题: %s\n", pr.Title)
	fmt.Fprintf(env.out, "   链接: %s\n", pr.HTMLURL)

	return nil
}
