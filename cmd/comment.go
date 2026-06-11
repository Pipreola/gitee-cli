// Package cmd 定义评论相关命令。
package cmd

import (
	"bufio"
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

// newPRCommentCmd 创建 pr comment 子命令。
func newPRCommentCmd() *cobra.Command {
	var (
		body     string
		bodyFile string
	)

	cmd := &cobra.Command{
		Use:   "comment <number>",
		Short: "为 Pull Request 添加评论",
		Long: `为指定的 Pull Request 添加评论。

如果不指定 --body 或 --body-file，将进入交互式输入模式。`,
		Example: `  # 交互式输入评论
  gitee pr comment 123

  # 使用 --body 直接指定评论内容
  gitee pr comment 123 --body "LGTM"

  # 从文件读取评论内容
  gitee pr comment 123 --body-file comment.txt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 解析 PR 编号
			number, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || number <= 0 {
				return fmt.Errorf("PR 编号必须是正整数")
			}

			opts := commentOptions{
				body:     body,
				bodyFile: bodyFile,
			}
			return runPRComment(context.Background(), number, opts, defaultCommentEnv())
		},
	}

	cmd.Flags().StringVarP(&body, "body", "b", "", "评论内容")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "从文件读取评论内容")

	return cmd
}

// newIssueCommentCmd 创建 issue comment 子命令。
func newIssueCommentCmd() *cobra.Command {
	var (
		body     string
		bodyFile string
	)

	cmd := &cobra.Command{
		Use:   "comment <number>",
		Short: "为 Issue 添加评论",
		Long: `为指定的 Issue 添加评论。

如果不指定 --body 或 --body-file，将进入交互式输入模式。`,
		Example: `  # 交互式输入评论
  gitee issue comment I123

  # 使用 --body 直接指定评论内容
  gitee issue comment I123 --body "已修复"

  # 从文件读取评论内容
  gitee issue comment I123 --body-file comment.txt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number := strings.TrimSpace(args[0])
			if number == "" {
				return fmt.Errorf("Issue 编号不能为空")
			}

			opts := commentOptions{
				body:     body,
				bodyFile: bodyFile,
			}
			return runIssueComment(context.Background(), number, opts, defaultCommentEnv())
		},
	}

	cmd.Flags().StringVarP(&body, "body", "b", "", "评论内容")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "从文件读取评论内容")

	return cmd
}

// commentOptions 收集评论命令的全部参数。
type commentOptions struct {
	body     string
	bodyFile string
}

// commentEnv 聚合评论命令的外部依赖。
type commentEnv struct {
	git                gitRunner
	loadConfig         func() (*config.Config, error)
	createPRComment    func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error)
	createIssueComment func(ctx context.Context, host, token, owner, repo, number string, input *api.CreateIssueCommentInput) (*api.Comment, error)
	readFile           func(filename string) ([]byte, error)
	in                 io.Reader
	out                io.Writer
}

// defaultCommentEnv 返回基于真实依赖的环境。
func defaultCommentEnv() commentEnv {
	return commentEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			client := api.NewClient(host, token)
			return client.CreatePullRequestComment(ctx, owner, repo, number, input)
		},
		createIssueComment: func(ctx context.Context, host, token, owner, repo, number string, input *api.CreateIssueCommentInput) (*api.Comment, error) {
			client := api.NewClient(host, token)
			return client.CreateIssueComment(ctx, owner, repo, number, input)
		},
		readFile: os.ReadFile,
		in:       os.Stdin,
		out:      os.Stdout,
	}
}

// runPRComment 执行 pr comment 的核心流程。
func runPRComment(ctx context.Context, number int64, opts commentOptions, env commentEnv) error {
	// 1. 加载配置，检查认证状态
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 2. 获取当前仓库信息
	owner, repo, err := getCurrentRepo(env.git)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	// 3. 获取评论内容
	body, err := getCommentBody(opts, env)
	if err != nil {
		return err
	}

	// 4. 调用 API 创建评论
	input := &api.CreatePullRequestCommentInput{
		Body: body,
	}

	comment, err := env.createPRComment(ctx, cfg.Host, cfg.Token, owner, repo, number, input)
	if err != nil {
		return fmt.Errorf("创建评论失败: %w", err)
	}

	// 5. 输出成功信息
	writeCommentResult(env.out, "PR", fmt.Sprintf("#%d", number), comment)

	return nil
}

// runIssueComment 执行 issue comment 的核心流程。
func runIssueComment(ctx context.Context, number string, opts commentOptions, env commentEnv) error {
	// 1. 加载配置，检查认证状态
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 2. 获取当前仓库信息
	owner, repo, err := getCurrentRepo(env.git)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	// 3. 获取评论内容
	body, err := getCommentBody(opts, env)
	if err != nil {
		return err
	}

	// 4. 调用 API 创建评论
	input := &api.CreateIssueCommentInput{
		Body: body,
	}

	comment, err := env.createIssueComment(ctx, cfg.Host, cfg.Token, owner, repo, number, input)
	if err != nil {
		return fmt.Errorf("创建评论失败: %w", err)
	}

	// 5. 输出成功信息
	writeCommentResult(env.out, "Issue", number, comment)

	return nil
}

// writeCommentResult 输出评论创建成功信息，包含评论编号与评论 URL（若有）。
// target 标明评论挂载的对象类型（PR / Issue），targetID 是其编号。
func writeCommentResult(w io.Writer, target, targetID string, comment *api.Comment) {
	fmt.Fprintf(w, "\n✅ 评论添加成功！\n")
	fmt.Fprintf(w, "   %s: %s\n", target, targetID)
	fmt.Fprintf(w, "   评论编号: %d\n", comment.ID)
	fmt.Fprintf(w, "   作者: %s\n", comment.User.Login)
	fmt.Fprintf(w, "   时间: %s\n", comment.CreatedAt)
	// Gitee 评论响应包含 html_url 时输出，方便用户直接跳转查看。
	if comment.HTMLURL != "" {
		fmt.Fprintf(w, "   链接: %s\n", comment.HTMLURL)
	}
	fmt.Fprintf(w, "   内容: %s\n", truncateString(comment.Body, 100))
}

// getCommentBody 根据 options 获取评论内容，优先级：body > bodyFile > 交互式输入。
func getCommentBody(opts commentOptions, env commentEnv) (string, error) {
	// 1. 如果 --body 和 --body-file 同时指定，报错
	if opts.body != "" && opts.bodyFile != "" {
		return "", fmt.Errorf("--body 和 --body-file 不能同时指定")
	}

	// 2. 优先使用 --body
	if opts.body != "" {
		return strings.TrimSpace(opts.body), nil
	}

	// 3. 其次使用 --body-file
	if opts.bodyFile != "" {
		content, err := env.readFile(opts.bodyFile)
		if err != nil {
			return "", fmt.Errorf("读取文件失败: %w", err)
		}
		body := strings.TrimSpace(string(content))
		if body == "" {
			return "", fmt.Errorf("文件内容不能为空")
		}
		return body, nil
	}

	// 4. 交互式输入（多行模式）
	fmt.Fprintln(env.out, "请输入评论内容（输入空行结束）:")
	scanner := bufio.NewScanner(env.in)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		// 空行表示结束输入
		if line == "" && len(lines) > 0 {
			break
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取输入失败: %w", err)
	}

	body := strings.TrimSpace(strings.Join(lines, "\n"))
	if body == "" {
		return "", fmt.Errorf("评论内容不能为空")
	}

	return body, nil
}

// truncateString 截断字符串到指定长度，超过部分用 ... 代替。
func truncateString(s string, maxLen int) string {
	// 移除换行符，将多行内容合并为一行显示
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// 合并多个空格为一个
	s = strings.Join(strings.Fields(s), " ")

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
