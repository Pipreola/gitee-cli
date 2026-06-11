// Package cmd 定义 PR（Pull Request）相关命令。
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// newPRCmd 创建 pr 命令。
func newPRCmd() *cobra.Command {
	prCmd := &cobra.Command{
		Use:   "pr",
		Short: "管理 Pull Request",
		Long:  "管理 Gitee Pull Request，包括创建、查看、列表等操作。",
	}

	prCmd.AddCommand(newPRCreateCmd())
	prCmd.AddCommand(newPRListCmd())
	prCmd.AddCommand(newPRViewCmd())
	prCmd.AddCommand(newPRCheckoutCmd())
	prCmd.AddCommand(newPRMergeCmd())
	return prCmd
}

// newPRCreateCmd 创建 pr create 子命令。
func newPRCreateCmd() *cobra.Command {
	var (
		title           string
		body            string
		base            string
		head            string
		draft           bool
		labels          string
		milestoneNumber int64
		assignees       string
		testers         string
		web             bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建 Pull Request",
		Long: `创建一个新的 Pull Request。

如果不指定 --title 和 --body，将进入交互式模式。
如果不指定 --head，将使用当前分支。
如果不指定 --base，将使用 main（若不存在则使用 master）。`,
		Example: `  # 交互式创建 PR
  gitee pr create

  # 指定标题和描述创建
  gitee pr create --title "Add new feature" --body "This PR adds..."

  # 创建草稿 PR
  gitee pr create --draft --title "WIP: Refactor auth"

  # 指定审阅者和标签
  gitee pr create --title "Fix bug" --assignees user1,user2 --labels bug,urgent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := prCreateOptions{
				title:           title,
				body:            body,
				bodyChanged:     cmd.Flags().Changed("body"),
				base:            base,
				head:            head,
				draft:           draft,
				labels:          labels,
				milestoneNumber: milestoneNumber,
				assignees:       assignees,
				testers:         testers,
				web:             web,
			}
			return runPRCreate(context.Background(), opts, defaultPRCreateEnv())
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "PR 标题")
	cmd.Flags().StringVarP(&body, "body", "b", "", "PR 描述")
	cmd.Flags().StringVar(&base, "base", "", "目标分支（默认 main 或 master）")
	cmd.Flags().StringVar(&head, "head", "", "源分支（默认当前分支）")
	cmd.Flags().BoolVarP(&draft, "draft", "d", false, "创建为草稿 PR")
	cmd.Flags().StringVarP(&labels, "labels", "l", "", "标签，逗号分隔")
	cmd.Flags().Int64VarP(&milestoneNumber, "milestone", "m", 0, "里程碑编号")
	cmd.Flags().StringVarP(&assignees, "assignees", "a", "", "审阅者，逗号分隔的用户名")
	cmd.Flags().StringVar(&testers, "testers", "", "测试者，逗号分隔的用户名")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "在浏览器中打开创建的 PR")

	return cmd
}

// gitRunner 抽象 git 命令执行，便于在测试中注入桩实现。
type gitRunner interface {
	// run 执行 git 子命令并返回合并后的输出。
	run(args ...string) (string, error)
	// runInteractive 执行 git 子命令并将标准输出/错误直连终端（用于 push）。
	runInteractive(args ...string) error
}

// execGitRunner 是基于 os/exec 的真实 git 执行实现。
type execGitRunner struct{}

func (execGitRunner) run(args ...string) (string, error) {
	output, err := exec.Command("git", args...).CombinedOutput()
	return string(output), err
}

func (execGitRunner) runInteractive(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// prCreateOptions 收集 pr create 子命令的全部参数。
type prCreateOptions struct {
	title           string
	body            string
	bodyChanged     bool
	base            string
	head            string
	draft           bool
	labels          string
	milestoneNumber int64
	assignees       string
	testers         string
	web             bool
}

// prCreateEnv 聚合 pr create 的外部依赖，使核心流程可在测试中完全注入。
type prCreateEnv struct {
	git         gitRunner
	loadConfig  func() (*config.Config, error)
	createPR    func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error)
	openBrowser func(url string) error
	in          io.Reader
	out         io.Writer
}

// defaultPRCreateEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultPRCreateEnv() prCreateEnv {
	return prCreateEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		createPR: func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.CreatePullRequest(ctx, owner, repo, input)
		},
		openBrowser: openBrowser,
		in:          os.Stdin,
		out:         os.Stdout,
	}
}

// runPRCreate 执行 pr create 的核心流程，所有外部依赖通过 env 注入。
func runPRCreate(ctx context.Context, opts prCreateOptions, env prCreateEnv) error {
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

	// 3. 获取当前分支（如果未指定 head）
	head := opts.head
	if head == "" {
		head, err = getCurrentBranch(env.git)
		if err != nil {
			return fmt.Errorf("获取当前分支失败: %w", err)
		}
	}

	// 4. 获取默认基础分支（如果未指定 base）
	base := opts.base
	if base == "" {
		base, err = getDefaultBaseBranch(env.git)
		if err != nil {
			return fmt.Errorf("获取默认基础分支失败: %w", err)
		}
	}

	// 5. 检查源分支和目标分支是否相同
	if head == base {
		return fmt.Errorf("源分支和目标分支不能相同（当前都是 %s）", head)
	}

	// 6. 确保分支已推送到远程
	if err := ensureBranchPushed(env, head); err != nil {
		return err
	}

	// 7. 交互式输入标题和描述（如果未指定）
	scanner := bufio.NewScanner(env.in)
	title := opts.title
	if title == "" {
		fmt.Fprint(env.out, "请输入 PR 标题: ")
		if scanner.Scan() {
			title = strings.TrimSpace(scanner.Text())
		}
		if title == "" {
			return fmt.Errorf("PR 标题不能为空")
		}
	}

	body := opts.body
	if body == "" && !opts.bodyChanged {
		fmt.Fprint(env.out, "请输入 PR 描述（可选，按回车跳过）: ")
		if scanner.Scan() {
			body = strings.TrimSpace(scanner.Text())
		}
	}

	// 8. 构造 API 请求
	input := &api.CreatePullRequestInput{
		Title:           title,
		Head:            head,
		Base:            base,
		Body:            body,
		Draft:           opts.draft,
		Labels:          opts.labels,
		MilestoneNumber: opts.milestoneNumber,
		Assignees:       opts.assignees,
		Testers:         opts.testers,
	}

	// 9. 调用 API 创建 PR
	pr, err := env.createPR(ctx, cfg.Host, cfg.Token, owner, repo, input)
	if err != nil {
		return fmt.Errorf("创建 PR 失败: %w", err)
	}

	// 10. 输出成功信息
	fmt.Fprintf(env.out, "\n✅ PR 创建成功！\n")
	fmt.Fprintf(env.out, "   标题: %s\n", pr.Title)
	fmt.Fprintf(env.out, "   编号: #%d\n", pr.Number)
	fmt.Fprintf(env.out, "   链接: %s\n", pr.HTMLURL)
	if pr.Body != "" {
		fmt.Fprintf(env.out, "   描述: %s\n", pr.Body)
	}
	if opts.draft {
		fmt.Fprintln(env.out, "   状态: 草稿")
	}

	// 11. 可选：在浏览器中打开
	if opts.web {
		if err := env.openBrowser(pr.HTMLURL); err != nil {
			fmt.Fprintf(env.out, "⚠️  无法自动打开浏览器: %v\n", err)
		}
	}

	return nil
}

// getCurrentRepo 从 git remote 获取当前仓库的 owner 和 repo 名称。
func getCurrentRepo(git gitRunner) (owner, repo string, err error) {
	// 获取远程仓库 URL
	output, err := git.run("remote", "get-url", "origin")
	if err != nil {
		return "", "", fmt.Errorf("无法获取远程仓库 URL: %w", err)
	}

	url := strings.TrimSpace(output)

	// 解析 URL，支持两种格式：
	// 1. https://gitee.com/owner/repo.git
	// 2. git@gitee.com:owner/repo.git
	if strings.HasPrefix(url, "https://") {
		// HTTPS 格式
		parts := strings.Split(strings.TrimPrefix(url, "https://gitee.com/"), "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("无法解析仓库 URL: %s", url)
		}
		owner = parts[0]
		repo = strings.TrimSuffix(parts[1], ".git")
	} else if strings.HasPrefix(url, "git@") {
		// SSH 格式
		parts := strings.Split(strings.TrimPrefix(url, "git@gitee.com:"), "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("无法解析仓库 URL: %s", url)
		}
		owner = parts[0]
		repo = strings.TrimSuffix(parts[1], ".git")
	} else {
		return "", "", fmt.Errorf("不支持的仓库 URL 格式: %s", url)
	}

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("无法从 URL 提取 owner 和 repo: %s", url)
	}

	return owner, repo, nil
}

// getCurrentBranch 获取当前 git 分支名称。
func getCurrentBranch(git gitRunner) (string, error) {
	output, err := git.run("branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("无法获取当前分支: %w", err)
	}
	branch := strings.TrimSpace(output)
	if branch == "" {
		return "", fmt.Errorf("当前不在任何分支上（可能处于 detached HEAD 状态）")
	}
	return branch, nil
}

// getDefaultBaseBranch 获取默认的基础分支（main 或 master）。
func getDefaultBaseBranch(git gitRunner) (string, error) {
	// 先检查 main 分支是否存在
	if _, err := git.run("rev-parse", "--verify", "origin/main"); err == nil {
		return "main", nil
	}

	// 再检查 master 分支
	if _, err := git.run("rev-parse", "--verify", "origin/master"); err == nil {
		return "master", nil
	}

	return "", fmt.Errorf("找不到默认分支（main 或 master），请使用 --base 指定目标分支")
}

// ensureBranchPushed 确保当前分支已推送到远程，如果未推送则自动推送。
func ensureBranchPushed(env prCreateEnv, branch string) error {
	// 检查远程分支是否存在
	output, err := env.git.run("ls-remote", "--heads", "origin", branch)
	if err != nil {
		return fmt.Errorf("检查远程分支失败: %w", err)
	}

	// 如果远程分支不存在，推送当前分支
	if strings.TrimSpace(output) == "" {
		fmt.Fprintf(env.out, "⚠️  分支 '%s' 尚未推送到远程，正在推送...\n", branch)
		if err := env.git.runInteractive("push", "-u", "origin", branch); err != nil {
			return fmt.Errorf("推送分支失败: %w", err)
		}
		fmt.Fprintln(env.out, "✅ 分支推送成功")
	}

	return nil
}

// openBrowser 在默认浏览器中打开 URL。
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch {
	case commandExists("xdg-open"):
		cmd = exec.Command("xdg-open", url)
	case commandExists("open"):
		cmd = exec.Command("open", url)
	case commandExists("start"):
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("无法检测到浏览器打开命令")
	}
	return cmd.Start()
}

// commandExists 检查命令是否存在。
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// newPRCheckoutCmd 创建 pr checkout 子命令。
func newPRCheckoutCmd() *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "checkout <number|url>",
		Short: "检出 Pull Request 到本地分支",
		Long: `检出一个 Pull Request 到本地分支。

可以通过以下方式指定 PR：
- PR 编号：gitee pr checkout 123
- PR URL：gitee pr checkout https://gitee.com/owner/repo/pulls/123

本地分支统一命名为 pr-<number>。如果本地分支已存在，默认以快进方式更新；
使用 --force 强制重置到 PR 最新提交（会丢弃该分支上的本地差异）。`,
		Example: `  # 检出 PR 编号 123
  gitee pr checkout 123

  # 检出 PR URL
  gitee pr checkout https://gitee.com/owner/repo/pulls/456

  # 强制更新已存在的分支
  gitee pr checkout 123 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := prCheckoutOptions{
				input: args[0],
				force: force,
			}
			return runPRCheckout(context.Background(), opts, defaultPRCheckoutEnv())
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "强制覆盖本地分支（即使有未提交的更改）")

	return cmd
}

// prCheckoutOptions 收集 pr checkout 子命令的参数。
type prCheckoutOptions struct {
	input string
	force bool
}

// prCheckoutEnv 聚合 pr checkout 的外部依赖。
type prCheckoutEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	getPR      func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error)
	out        io.Writer
}

// defaultPRCheckoutEnv 返回基于真实依赖的环境。
func defaultPRCheckoutEnv() prCheckoutEnv {
	return prCheckoutEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.GetPullRequest(ctx, owner, repo, number)
		},
		out: os.Stdout,
	}
}

// runPRCheckout 执行 pr checkout 的核心流程。
func runPRCheckout(ctx context.Context, opts prCheckoutOptions, env prCheckoutEnv) error {
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

	// 3. 解析输入，确定 PR 编号
	prNumber, err := parsePRInput(opts.input, owner, repo)
	if err != nil {
		return fmt.Errorf("解析 PR 输入失败: %w", err)
	}

	// 4. 调用 API 获取 PR 详情
	pr, err := env.getPR(ctx, cfg.Host, cfg.Token, owner, repo, prNumber)
	if err != nil {
		return fmt.Errorf("获取 PR 详情失败: %w", err)
	}

	// 5. 提取分支信息
	headLabel := pr.Head.Label // 格式: namespace:branch 或 branch
	baseBranch := pr.Base.Ref

	fmt.Fprintf(env.out, "检出 PR #%d: %s\n", pr.Number, pr.Title)
	fmt.Fprintf(env.out, "  源分支: %s\n", headLabel)
	fmt.Fprintf(env.out, "  目标分支: %s\n", baseBranch)
	fmt.Fprintf(env.out, "  状态: %s\n", pr.State)

	// 6. 警告：PR 已关闭或合并
	if pr.State == "closed" {
		if pr.MergedAt != "" {
			fmt.Fprintf(env.out, "⚠️  该 PR 已合并\n")
		} else {
			fmt.Fprintf(env.out, "⚠️  该 PR 已关闭\n")
		}
	}

	// 7. 检查本地是否有未提交的更改（非 force 模式）
	if !opts.force {
		output, err := env.git.run("status", "--porcelain")
		if err != nil {
			return fmt.Errorf("检查本地状态失败: %w", err)
		}
		if strings.TrimSpace(output) != "" {
			return fmt.Errorf("本地有未提交的更改，请先提交或使用 --force 强制检出")
		}
	}

	// 8. 确定本地分支名
	localBranch := fmt.Sprintf("pr-%d", pr.Number)

	// 9. 先把 PR 的远程 head 拉取到 FETCH_HEAD。
	//    注意：不要直接 fetch 到本地分支引用（pull/N/head:pr-N），
	//    因为当 pr-N 正是当前检出分支时，真实 git 会拒绝 fetch 到该引用。
	//    统一拉到 FETCH_HEAD 后再决定如何更新本地分支，符合真实 git 语义。
	fetchRef := fmt.Sprintf("pull/%d/head", pr.Number)
	fmt.Fprintf(env.out, "正在拉取 PR 分支...\n")
	if _, err := env.git.run("fetch", "origin", fetchRef); err != nil {
		return fmt.Errorf("拉取 PR 分支失败: %w", err)
	}

	// 10. 检查本地分支是否已存在
	_, err = env.git.run("rev-parse", "--verify", localBranch)
	branchExists := err == nil

	if branchExists {
		// 已存在：切换到该分支，再用 FETCH_HEAD 更新
		fmt.Fprintf(env.out, "本地分支 '%s' 已存在，正在切换并更新...\n", localBranch)
		if _, err := env.git.run("checkout", localBranch); err != nil {
			return fmt.Errorf("切换分支失败: %w", err)
		}
		if opts.force {
			// force 模式：强制重置到 PR 最新提交，丢弃本地差异
			if _, err := env.git.run("reset", "--hard", "FETCH_HEAD"); err != nil {
				return fmt.Errorf("重置分支失败: %w", err)
			}
		} else {
			// 非 force：仅允许快进更新，避免覆盖本地提交
			if _, err := env.git.run("merge", "--ff-only", "FETCH_HEAD"); err != nil {
				return fmt.Errorf("更新分支失败（非快进），请使用 --force 强制覆盖: %w", err)
			}
		}
	} else {
		// 不存在：基于 FETCH_HEAD 创建并切换到新分支
		fmt.Fprintf(env.out, "创建并切换到新分支 '%s'...\n", localBranch)
		if _, err := env.git.run("checkout", "-b", localBranch, "FETCH_HEAD"); err != nil {
			return fmt.Errorf("创建分支失败: %w", err)
		}
	}

	// 11. 输出成功信息
	fmt.Fprintf(env.out, "\n✅ PR #%d 检出成功！\n", pr.Number)
	fmt.Fprintf(env.out, "   本地分支: %s\n", localBranch)
	fmt.Fprintf(env.out, "   PR 链接: %s\n", pr.HTMLURL)

	return nil
}

// parsePRInput 解析用户输入，支持 PR 编号或 URL，返回 PR 编号。
// 暂不支持通过分支名检出（需要额外的 list PR API 才能定位编号）。
func parsePRInput(input, owner, repo string) (int64, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("输入不能为空")
	}

	// 情况1: 纯数字，直接作为 PR 编号
	var prNumber int64
	if _, err := fmt.Sscanf(input, "%d", &prNumber); err == nil && prNumber > 0 {
		return prNumber, nil
	}

	// 情况2: URL 格式
	// https://gitee.com/owner/repo/pulls/123
	if strings.HasPrefix(input, "https://gitee.com/") || strings.HasPrefix(input, "http://gitee.com/") {
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(input, "https://gitee.com/"), "http://gitee.com/"), "/")
		if len(parts) >= 4 && parts[2] == "pulls" {
			if _, err := fmt.Sscanf(parts[3], "%d", &prNumber); err == nil && prNumber > 0 {
				return prNumber, nil
			}
		}
		return 0, fmt.Errorf("无法从 URL 中解析 PR 编号: %s", input)
	}

	// 情况3: 其他（含分支名）暂不支持
	return 0, fmt.Errorf("暂不支持通过分支名检出，请使用 PR 编号或 URL")
}

// newPRMergeCmd 创建 pr merge 子命令。
func newPRMergeCmd() *cobra.Command {
	var (
		method       string
		message      string
		deleteBranch bool
	)

	cmd := &cobra.Command{
		Use:   "merge <number>",
		Short: "合并 Pull Request",
		Long: `合并一个 Pull Request。

支持三种合并方式：
- merge（默认）：标准合并，保留所有提交历史
- squash：压缩合并，将所有提交合并为一个
- rebase：变基合并，在目标分支上重放提交

合并前会自动校验 PR 状态，确保 PR 处于 open 状态且可合并。`,
		Example: `  # 使用默认方式（merge）合并 PR
  gitee pr merge 123

  # 使用 squash 方式合并
  gitee pr merge 123 --method squash

  # 合并后删除源分支
  gitee pr merge 123 --delete-branch

  # 自定义合并信息
  gitee pr merge 123 --message "Merge feature X into main"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := prMergeOptions{
				input:        args[0],
				method:       method,
				message:      message,
				deleteBranch: deleteBranch,
			}
			return runPRMerge(context.Background(), opts, defaultPRMergeEnv())
		},
	}

	cmd.Flags().StringVar(&method, "method", "merge", "合并方式：merge（默认）/ squash / rebase")
	cmd.Flags().StringVar(&message, "message", "", "自定义合并提交信息")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "合并后删除源分支")

	return cmd
}

// prMergeOptions 收集 pr merge 子命令的参数。
type prMergeOptions struct {
	input        string
	method       string
	message      string
	deleteBranch bool
}

// prMergeEnv 聚合 pr merge 的外部依赖。
type prMergeEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	getPR      func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error)
	mergePR    func(ctx context.Context, host, token, owner, repo string, number int64, input *api.MergePullRequestInput) error
	out        io.Writer
}

// defaultPRMergeEnv 返回基于真实依赖的环境。
func defaultPRMergeEnv() prMergeEnv {
	return prMergeEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.GetPullRequest(ctx, owner, repo, number)
		},
		mergePR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.MergePullRequestInput) error {
			client := api.NewClient(host, token)
			return client.MergePullRequest(ctx, owner, repo, number, input)
		},
		out: os.Stdout,
	}
}

// runPRMerge 执行 pr merge 的核心流程。
func runPRMerge(ctx context.Context, opts prMergeOptions, env prMergeEnv) error {
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

	// 3. 解析输入，确定 PR 编号
	prNumber, err := parsePRInput(opts.input, owner, repo)
	if err != nil {
		return fmt.Errorf("解析 PR 输入失败: %w", err)
	}

	// 4. 调用 API 获取 PR 详情，校验状态
	pr, err := env.getPR(ctx, cfg.Host, cfg.Token, owner, repo, prNumber)
	if err != nil {
		return fmt.Errorf("获取 PR 详情失败: %w", err)
	}

	// 5. 校验 PR 状态
	if pr.State != "open" {
		return fmt.Errorf("PR #%d 状态为 %s，无法合并（只能合并状态为 open 的 PR）", pr.Number, pr.State)
	}

	if !pr.Mergeable {
		return fmt.Errorf("PR #%d 当前不可合并，可能存在冲突或其他阻塞条件", pr.Number)
	}

	// 6. 校验合并方式参数
	validMethods := map[string]bool{"merge": true, "squash": true, "rebase": true}
	if !validMethods[opts.method] {
		return fmt.Errorf("无效的合并方式 '%s'，支持：merge / squash / rebase", opts.method)
	}

	// 7. 构造合并请求
	// --message 对应 Gitee v5 API 的 description 字段（formData）。
	input := &api.MergePullRequestInput{
		MergeMethod:       opts.method,
		Description:       opts.message,
		PruneSourceBranch: opts.deleteBranch,
	}

	// 8. 执行合并
	fmt.Fprintf(env.out, "正在合并 PR #%d: %s\n", pr.Number, pr.Title)
	fmt.Fprintf(env.out, "  合并方式: %s\n", opts.method)
	if opts.deleteBranch {
		fmt.Fprintf(env.out, "  合并后将删除源分支: %s\n", pr.Head.Ref)
	}

	if err := env.mergePR(ctx, cfg.Host, cfg.Token, owner, repo, prNumber, input); err != nil {
		return fmt.Errorf("合并 PR 失败: %w", err)
	}

	// 9. 输出成功信息
	fmt.Fprintf(env.out, "\n✅ PR #%d 合并成功！\n", pr.Number)
	fmt.Fprintf(env.out, "   标题: %s\n", pr.Title)
	fmt.Fprintf(env.out, "   链接: %s\n", pr.HTMLURL)

	return nil
}
