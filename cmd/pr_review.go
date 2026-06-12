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

// newPRReviewCmd 创建 pr review 子命令。
//
// 该命令支持两类审查动作（二选一，必须显式指定其一）：
//   - --approve：调用 Gitee 审查接口将 PR 标记为「审查通过」；可叠加 --body 同时附一条评论。
//   - --comment：仅对 PR 发表一条评论（必须配合 --body 提供内容）。
func newPRReviewCmd() *cobra.Command {
	var (
		approve  bool
		comment  bool
		body     string
		bodyFile string
		force    bool
	)

	cmd := &cobra.Command{
		Use:   "review <number>",
		Short: "审查 Pull Request",
		Long: `审查指定编号的 Pull Request。

必须指定以下动作之一：
  --approve   将 PR 标记为「审查通过」，可同时使用 --body 附加一条评论
  --comment   仅对 PR 发表评论（需配合 --body 或 --body-file 提供评论内容）

--approve 与 --comment 不能同时指定。`,
		Example: `  # 审查通过 PR #123
  gitee pr review 123 --approve

  # 审查通过并附带评论
  gitee pr review 123 --approve --body "LGTM，逻辑清晰"

  # 强制通过（忽略分支保护的审查/测试规则限制）
  gitee pr review 123 --approve --force

  # 仅发表审查评论
  gitee pr review 123 --comment --body "第 10 行建议补充边界校验"

  # 从文件读取评论内容
  gitee pr review 123 --comment --body-file review.txt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || number <= 0 {
				return fmt.Errorf("无效的 PR 编号 %q，必须是正整数", args[0])
			}

			opts := prReviewOptions{
				approve:  approve,
				comment:  comment,
				body:     body,
				bodyFile: bodyFile,
				force:    force,
			}
			return runPRReview(context.Background(), number, opts, defaultPRReviewEnv())
		},
	}

	cmd.Flags().BoolVar(&approve, "approve", false, "将 PR 标记为审查通过")
	cmd.Flags().BoolVar(&comment, "comment", false, "仅发表审查评论（需配合 --body / --body-file）")
	cmd.Flags().StringVarP(&body, "body", "b", "", "评论内容")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "从文件读取评论内容")
	cmd.Flags().BoolVar(&force, "force", false, "强制通过，忽略分支保护的审查/测试规则限制（仅对 --approve 生效）")

	return cmd
}

// prReviewOptions 收集 pr review 子命令的全部参数。
type prReviewOptions struct {
	approve  bool
	comment  bool
	body     string
	bodyFile string
	force    bool
}

// prReviewEnv 聚合 pr review 的外部依赖，使核心流程可在测试中完全注入。
type prReviewEnv struct {
	git             gitRunner
	loadConfig      func() (*config.Config, error)
	reviewPR        func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error
	createPRComment func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error)
	readFile        func(filename string) ([]byte, error)
	out             io.Writer
}

// defaultPRReviewEnv 返回基于真实依赖的环境。
func defaultPRReviewEnv() prReviewEnv {
	return prReviewEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		reviewPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
			client := api.NewClient(host, token)
			return client.ReviewPullRequest(ctx, owner, repo, number, input)
		},
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			client := api.NewClient(host, token)
			return client.CreatePullRequestComment(ctx, owner, repo, number, input)
		},
		readFile: os.ReadFile,
		out:      os.Stdout,
	}
}

// runPRReview 执行 pr review 的核心流程，所有外部依赖通过 env 注入。
func runPRReview(ctx context.Context, number int64, opts prReviewOptions, env prReviewEnv) error {
	// 1. 校验动作参数：--approve 与 --comment 二选一，且必须指定其一。
	if opts.approve && opts.comment {
		return fmt.Errorf("--approve 和 --comment 不能同时指定")
	}
	if !opts.approve && !opts.comment {
		return fmt.Errorf("必须指定审查动作：--approve（审查通过）或 --comment（发表评论）")
	}

	// 2. 解析评论内容（--body 优先于 --body-file）。
	//    - --comment 模式下评论内容必填；
	//    - --approve 模式下评论内容可选（提供则在审查通过后附加一条评论）。
	reviewBody, err := resolveReviewBody(opts, env)
	if err != nil {
		return err
	}
	if opts.comment && reviewBody == "" {
		return fmt.Errorf("--comment 模式必须通过 --body 或 --body-file 提供评论内容")
	}
	if opts.comment && opts.force {
		return fmt.Errorf("--force 仅对 --approve 生效，不能与 --comment 同时使用")
	}

	// 3. 加载配置，检查认证状态。
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 4. 获取当前仓库信息。
	owner, repo, err := getCurrentRepo(env.git)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	// 5. 执行审查动作。
	if opts.approve {
		input := &api.ReviewPullRequestInput{Force: opts.force}
		if err := env.reviewPR(ctx, cfg.Host, cfg.Token, owner, repo, number, input); err != nil {
			return fmt.Errorf("审查通过失败: %w", err)
		}
		fmt.Fprintf(env.out, "\n✅ PR #%d 已审查通过！\n", number)
		if opts.force {
			fmt.Fprintln(env.out, "   模式: 强制通过（已忽略分支保护规则）")
		}

		// --approve 可选附带一条评论。
		if reviewBody != "" {
			commentInput := &api.CreatePullRequestCommentInput{Body: reviewBody}
			c, err := env.createPRComment(ctx, cfg.Host, cfg.Token, owner, repo, number, commentInput)
			if err != nil {
				return fmt.Errorf("审查已通过，但附加评论失败: %w", err)
			}
			writeCommentResult(env.out, "PR", fmt.Sprintf("#%d", number), c)
		}
		return nil
	}

	// --comment：仅发表评论。
	commentInput := &api.CreatePullRequestCommentInput{Body: reviewBody}
	c, err := env.createPRComment(ctx, cfg.Host, cfg.Token, owner, repo, number, commentInput)
	if err != nil {
		return fmt.Errorf("发表审查评论失败: %w", err)
	}
	writeCommentResult(env.out, "PR", fmt.Sprintf("#%d", number), c)

	return nil
}

// resolveReviewBody 根据 options 解析评论内容，优先级：--body > --body-file。
// 两者均未指定时返回空字符串（由调用方按动作判断是否必填）。
func resolveReviewBody(opts prReviewOptions, env prReviewEnv) (string, error) {
	if opts.body != "" && opts.bodyFile != "" {
		return "", fmt.Errorf("--body 和 --body-file 不能同时指定")
	}

	if opts.body != "" {
		return strings.TrimSpace(opts.body), nil
	}

	if opts.bodyFile != "" {
		content, err := env.readFile(opts.bodyFile)
		if err != nil {
			return "", fmt.Errorf("读取文件失败: %w", err)
		}
		text := strings.TrimSpace(string(content))
		if text == "" {
			return "", fmt.Errorf("文件内容不能为空")
		}
		return text, nil
	}

	return "", nil
}
