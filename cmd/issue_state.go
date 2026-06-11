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

// newIssueCloseCmd 创建 issue close 子命令。
func newIssueCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <number>",
		Short: "关闭 Issue",
		Long: `关闭指定编号的 Issue。

操作后会回显 Issue 的新状态。`,
		Example: `  # 关闭 Issue I123
  gitee issue close I123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number := strings.TrimSpace(args[0])
			if number == "" {
				return fmt.Errorf("Issue 编号不能为空")
			}
			return runIssueUpdateState(context.Background(), number, "closed", defaultIssueStateEnv())
		},
	}
	return cmd
}

// newIssueReopenCmd 创建 issue reopen 子命令。
func newIssueReopenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reopen <number>",
		Short: "重新打开 Issue",
		Long: `重新打开已关闭的 Issue。

操作后会回显 Issue 的新状态。`,
		Example: `  # 重新打开 Issue I123
  gitee issue reopen I123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number := strings.TrimSpace(args[0])
			if number == "" {
				return fmt.Errorf("Issue 编号不能为空")
			}
			return runIssueUpdateState(context.Background(), number, "open", defaultIssueStateEnv())
		},
	}
	return cmd
}

// issueStateEnv 聚合 issue close/reopen 的外部依赖。
type issueStateEnv struct {
	git               gitRunner
	loadConfig        func() (*config.Config, error)
	updateIssueState  func(ctx context.Context, host, token, owner, repo, number, state string) (*api.Issue, error)
	out               io.Writer
}

// defaultIssueStateEnv 返回基于真实依赖的环境。
func defaultIssueStateEnv() issueStateEnv {
	return issueStateEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		updateIssueState: func(ctx context.Context, host, token, owner, repo, number, state string) (*api.Issue, error) {
			client := api.NewClient(host, token)
			return client.UpdateIssueState(ctx, owner, repo, number, state)
		},
		out: os.Stdout,
	}
}

// runIssueUpdateState 执行 Issue 状态更新的核心流程。
func runIssueUpdateState(ctx context.Context, number, state string, env issueStateEnv) error {
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

	// 3. 调用 API 更新状态
	issue, err := env.updateIssueState(ctx, cfg.Host, cfg.Token, owner, repo, number, state)
	if err != nil {
		return fmt.Errorf("更新 Issue 状态失败: %w", err)
	}

	// 4. 输出成功信息
	var action string
	if state == "closed" {
		action = "关闭"
	} else {
		action = "重新打开"
	}
	fmt.Fprintf(env.out, "✅ Issue %s 已%s\n", issue.Number, action)
	fmt.Fprintf(env.out, "   标题: %s\n", issue.Title)
	fmt.Fprintf(env.out, "   状态: %s\n", strings.ToUpper(issue.State))
	fmt.Fprintf(env.out, "   链接: %s\n", issue.HTMLURL)

	return nil
}
