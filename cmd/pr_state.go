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

// newPRCloseCmd 创建 pr close 子命令。
func newPRCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <number>",
		Short: "关闭 Pull Request",
		Long: `关闭指定编号的 Pull Request。

操作后会回显 PR 的新状态。`,
		Example: `  # 关闭 PR #123
  gitee pr close 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || number <= 0 {
				return fmt.Errorf("无效的 PR 编号 %q，必须是正整数", args[0])
			}
			return runPRUpdateState(context.Background(), number, "closed", defaultPRStateEnv())
		},
	}
	return cmd
}

// newPRReopenCmd 创建 pr reopen 子命令。
func newPRReopenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reopen <number>",
		Short: "重新打开 Pull Request",
		Long: `重新打开已关闭的 Pull Request。

操作后会回显 PR 的新状态。`,
		Example: `  # 重新打开 PR #123
  gitee pr reopen 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || number <= 0 {
				return fmt.Errorf("无效的 PR 编号 %q，必须是正整数", args[0])
			}
			return runPRUpdateState(context.Background(), number, "open", defaultPRStateEnv())
		},
	}
	return cmd
}

// prStateEnv 聚合 pr close/reopen 的外部依赖。
type prStateEnv struct {
	git             gitRunner
	loadConfig      func() (*config.Config, error)
	updatePRState   func(ctx context.Context, host, token, owner, repo string, number int64, state string) (*api.PullRequest, error)
	out             io.Writer
}

// defaultPRStateEnv 返回基于真实依赖的环境。
func defaultPRStateEnv() prStateEnv {
	return prStateEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		updatePRState: func(ctx context.Context, host, token, owner, repo string, number int64, state string) (*api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.UpdatePullRequestState(ctx, owner, repo, number, state)
		},
		out: os.Stdout,
	}
}

// runPRUpdateState 执行 PR 状态更新的核心流程。
func runPRUpdateState(ctx context.Context, number int64, state string, env prStateEnv) error {
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
	pr, err := env.updatePRState(ctx, cfg.Host, cfg.Token, owner, repo, number, state)
	if err != nil {
		return fmt.Errorf("更新 PR 状态失败: %w", err)
	}

	// 4. 输出成功信息
	var action string
	if state == "closed" {
		action = "关闭"
	} else {
		action = "重新打开"
	}
	fmt.Fprintf(env.out, "✅ PR #%d 已%s\n", pr.Number, action)
	fmt.Fprintf(env.out, "   标题: %s\n", pr.Title)
	fmt.Fprintf(env.out, "   状态: %s\n", strings.ToUpper(pr.State))
	fmt.Fprintf(env.out, "   链接: %s\n", pr.HTMLURL)

	return nil
}
