package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// issueCreateOptions 收集 issue create 子命令的全部参数。
type issueCreateOptions struct {
	title           string
	body            string
	bodyFile        string
	bodyChanged     bool
	labels          string
	assignees       string
	milestoneNumber int64
	web             bool
}

// issueCreateEnv 聚合 issue create 的外部依赖，使核心流程可在测试中完全注入。
type issueCreateEnv struct {
	git         gitRunner
	loadConfig  func() (*config.Config, error)
	createIssue func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error)
	openBrowser func(url string) error
	readFile    func(path string) ([]byte, error)
	in          io.Reader
	out         io.Writer
}

// defaultIssueCreateEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultIssueCreateEnv() issueCreateEnv {
	return issueCreateEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			client := api.NewClient(host, token)
			return client.CreateIssue(ctx, owner, repo, input)
		},
		openBrowser: openURL,
		readFile:    os.ReadFile,
		in:          os.Stdin,
		out:         os.Stdout,
	}
}

// newIssueCreateCmd 创建 issue create 子命令。
func newIssueCreateCmd() *cobra.Command {
	var (
		title           string
		body            string
		bodyFile        string
		labels          string
		assignees       string
		milestoneNumber int64
		web             bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建 Issue",
		Long: `创建一个新的 Issue。

如果不指定 --title，将进入交互式模式。
--body 和 --body-file 二选一，--body-file 从文件读取描述内容。`,
		Example: `  # 交互式创建 Issue
  gitee issue create

  # 指定标题和描述创建
  gitee issue create --title "修复登录问题" --body "详细描述..."

  # 从文件读取描述
  gitee issue create --title "添加新功能" --body-file description.md

  # 指定标签和指派人
  gitee issue create --title "紧急Bug" --labels bug,urgent --assignees user1,user2

  # 创建后在浏览器中打开
  gitee issue create --title "新需求" --web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := issueCreateOptions{
				title:           title,
				body:            body,
				bodyFile:        bodyFile,
				bodyChanged:     cmd.Flags().Changed("body"),
				labels:          labels,
				assignees:       assignees,
				milestoneNumber: milestoneNumber,
				web:             web,
			}
			return runIssueCreate(context.Background(), opts, defaultIssueCreateEnv())
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "Issue 标题（必填，未指定时进入交互式输入）")
	cmd.Flags().StringVarP(&body, "body", "b", "", "Issue 描述")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "从文件读取 Issue 描述")
	cmd.Flags().StringVarP(&labels, "labels", "l", "", "标签，逗号分隔")
	cmd.Flags().StringVarP(&assignees, "assignees", "a", "", "指派人，逗号分隔的用户名")
	cmd.Flags().Int64VarP(&milestoneNumber, "milestone", "m", 0, "里程碑编号")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "在浏览器中打开创建的 Issue")

	cmd.MarkFlagsMutuallyExclusive("body", "body-file")

	return cmd
}

// runIssueCreate 执行 issue create 的核心流程，所有外部依赖通过 env 注入。
func runIssueCreate(ctx context.Context, opts issueCreateOptions, env issueCreateEnv) error {
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

	// 3. 处理 --body-file
	body := opts.body
	if opts.bodyFile != "" {
		content, err := env.readFile(opts.bodyFile)
		if err != nil {
			return fmt.Errorf("读取文件 %q 失败: %w", opts.bodyFile, err)
		}
		body = string(content)
	}

	// 4. 交互式输入标题和描述（如果未指定）
	scanner := bufio.NewScanner(env.in)
	title := opts.title
	if title == "" {
		fmt.Fprint(env.out, "请输入 Issue 标题: ")
		if scanner.Scan() {
			title = strings.TrimSpace(scanner.Text())
		}
		if title == "" {
			return fmt.Errorf("Issue 标题不能为空")
		}
	}

	if body == "" && !opts.bodyChanged && opts.bodyFile == "" {
		fmt.Fprint(env.out, "请输入 Issue 描述（可选，按回车跳过）: ")
		if scanner.Scan() {
			body = strings.TrimSpace(scanner.Text())
		}
	}

	// 5. 构造 API 请求
	input := &api.CreateIssueInput{
		Title:           title,
		Body:            body,
		Labels:          opts.labels,
		Assignees:       opts.assignees,
		MilestoneNumber: opts.milestoneNumber,
	}

	// 6. 调用 API 创建 Issue
	issue, err := env.createIssue(ctx, cfg.Host, cfg.Token, owner, repo, input)
	if err != nil {
		return fmt.Errorf("创建 Issue 失败: %w", err)
	}

	// 7. 输出成功信息
	fmt.Fprintf(env.out, "\n✅ Issue 创建成功！\n")
	fmt.Fprintf(env.out, "   标题: %s\n", issue.Title)
	fmt.Fprintf(env.out, "   编号: %s\n", issue.Number)
	fmt.Fprintf(env.out, "   链接: %s\n", issue.HTMLURL)
	if issue.Body != "" {
		fmt.Fprintf(env.out, "   描述: %s\n", issue.Body)
	}
	if len(issue.Labels) > 0 {
		labelNames := formatIssueLabels(issue.Labels)
		fmt.Fprintf(env.out, "   标签: %s\n", labelNames)
	}
	if issue.Assignee != nil {
		fmt.Fprintf(env.out, "   指派给: %s\n", issue.Assignee.Login)
	}

	// 8. 可选：在浏览器中打开
	if opts.web {
		if err := env.openBrowser(issue.HTMLURL); err != nil {
			fmt.Fprintf(env.out, "⚠️  无法自动打开浏览器: %v\n", err)
		}
	}

	return nil
}
