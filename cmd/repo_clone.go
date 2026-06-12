package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/config"
)

// repoCloneOptions 收集 repo clone 子命令的参数。
type repoCloneOptions struct {
	// repo 是 [owner/]repo 形式的仓库定位参数（必填）。
	repo string
	// dir 是可选的克隆目标目录；为空时由 git 按仓库名创建。
	dir string
	// gitArgs 是透传给底层 git clone 的额外参数（如 --depth 1）。
	gitArgs []string
}

// repoCloneEnv 聚合 repo clone 的外部依赖，使核心流程可在测试中完全注入。
type repoCloneEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	out        io.Writer
}

// defaultRepoCloneEnv 返回基于真实 git / 配置的依赖集合。
func defaultRepoCloneEnv() repoCloneEnv {
	return repoCloneEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		out:        os.Stdout,
	}
}

// newRepoCloneCmd 创建 repo clone 子命令。
func newRepoCloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <owner/repo> [dir] [-- <git-clone-args>...]",
		Short: "克隆仓库到本地（含鉴权）",
		Long: `使用本地保存的认证令牌克隆 Gitee 仓库到本地。

仓库定位支持 owner/repo 完整形式，或仅 repo（owner 取当前登录用户）。
可选地指定目标目录；位于 -- 之后的参数会原样透传给底层 git clone，
例如 --depth、--branch、--single-branch 等。

为避免令牌泄露，克隆完成后会自动将 origin 远程地址重置为不含令牌的干净 URL。`,
		Example: `  # 克隆仓库
  gitee repo clone owner/repo

  # 克隆到指定目录
  gitee repo clone owner/repo my-dir

  # 浅克隆（透传 --depth 给 git）
  gitee repo clone owner/repo -- --depth 1

  # 克隆指定分支
  gitee repo clone owner/repo -- --branch dev --single-branch`,
		Args: cobra.ArbitraryArgs,
		// 禁用 cobra 对未知 flag 的解析，使 --depth 等参数可原样透传给 git。
		// 位置参数与透传参数以 "--" 分隔。
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := parseRepoCloneArgs(cmd, args)
			if err != nil {
				return err
			}
			return runRepoClone(opts, defaultRepoCloneEnv())
		},
	}

	return cmd
}

// parseRepoCloneArgs 将命令行参数拆分为位置参数（repo、dir）与 "--" 之后的 git 透传参数。
func parseRepoCloneArgs(cmd *cobra.Command, args []string) (repoCloneOptions, error) {
	// ArgsLenAtDash 返回 "--" 之前的位置参数个数；为 -1 表示命令行中没有 "--"。
	return parseRepoCloneArgsWithDash(cmd, args, cmd.ArgsLenAtDash())
}

// parseRepoCloneArgsWithDash 是 parseRepoCloneArgs 的核心实现，显式接收 "--" 位置索引，便于测试。
func parseRepoCloneArgsWithDash(_ *cobra.Command, args []string, dashIdx int) (repoCloneOptions, error) {
	opts := repoCloneOptions{}

	var positional []string
	if dashIdx >= 0 {
		positional = args[:dashIdx]
		opts.gitArgs = args[dashIdx:]
	} else {
		positional = args
	}

	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少仓库参数，用法: gitee repo clone <owner/repo> [dir]")
	}
	if len(positional) > 2 {
		return opts, fmt.Errorf("位置参数过多，仅支持 <owner/repo> 与可选的目标目录；git 透传参数请放在 -- 之后")
	}

	opts.repo = positional[0]
	if len(positional) == 2 {
		opts.dir = positional[1]
	}
	return opts, nil
}

// runRepoClone 执行 repo clone 的核心流程。
func runRepoClone(opts repoCloneOptions, env repoCloneEnv) error {
	// 1. 加载配置，检查认证
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 2. 解析 owner/repo。仅给出 repo 时，owner 取当前登录用户。
	owner, repo, err := resolveCloneTarget(opts.repo, cfg.User)
	if err != nil {
		return err
	}

	// 3. 派生 git 主机地址，构造干净 URL 与带令牌的鉴权 URL。
	host, err := deriveGitHost(cfg.Host)
	if err != nil {
		return err
	}
	cleanURL := fmt.Sprintf("%s/%s/%s.git", host, owner, repo)
	authURL := buildAuthCloneURL(host, owner, repo, cfg.Token)

	// 4. 组装 git clone 参数：clone [git透传参数] <authURL> [dir]
	cloneArgs := []string{"clone"}
	cloneArgs = append(cloneArgs, opts.gitArgs...)
	cloneArgs = append(cloneArgs, authURL)
	if opts.dir != "" {
		cloneArgs = append(cloneArgs, opts.dir)
	}

	fmt.Fprintf(env.out, "正在克隆 %s/%s ...\n", owner, repo)
	if err := env.git.runInteractive(cloneArgs...); err != nil {
		return fmt.Errorf("克隆仓库失败: %w", err)
	}

	// 5. 重置 origin 为不含令牌的干净 URL，避免令牌持久化到 .git/config。
	//    该步骤为尽力而为：失败仅告警，不影响克隆结果。
	targetDir := cloneTargetDir(opts.dir, repo)
	if _, err := env.git.run("-C", targetDir, "remote", "set-url", "origin", cleanURL); err != nil {
		fmt.Fprintf(env.out, "⚠️  无法重置 origin 远程地址（令牌可能残留在 .git/config）: %v\n", err)
	}

	fmt.Fprintf(env.out, "✅ 已克隆到 %s\n", targetDir)
	return nil
}

// resolveCloneTarget 解析仓库定位参数：
// 给出 owner/repo 时直接拆分；仅给出 repo 时 owner 取当前登录用户。
func resolveCloneTarget(arg, currentUser string) (owner, repo string, err error) {
	arg = trimGitSuffix(strings.TrimSpace(arg))
	if arg == "" {
		return "", "", fmt.Errorf("仓库参数不能为空")
	}

	if strings.Contains(arg, "/") {
		return parseRepoArg(arg)
	}

	// 仅给出 repo：使用当前登录用户作为 owner。
	if currentUser == "" {
		return "", "", fmt.Errorf("仅指定了仓库名 %q，但无法确定 owner（当前配置未记录登录用户），请使用 owner/repo 形式", arg)
	}
	return currentUser, arg, nil
}

// deriveGitHost 从 API Host（如 https://gitee.com/api/v5）派生 git 主机地址（如 https://gitee.com）。
func deriveGitHost(apiHost string) (string, error) {
	h := strings.TrimSpace(apiHost)
	if h == "" {
		h = config.DefaultHost
	}
	u, err := url.Parse(h)
	if err != nil {
		return "", fmt.Errorf("无法解析配置中的 Host %q: %w", apiHost, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("配置中的 Host %q 格式无效，期望形如 https://gitee.com/api/v5", apiHost)
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), nil
}

// buildAuthCloneURL 构造带令牌的 HTTPS clone URL。
// Gitee 支持 https://oauth2:<token>@host/owner/repo.git 形式进行令牌鉴权。
func buildAuthCloneURL(host, owner, repo, token string) string {
	u, err := url.Parse(host)
	if err != nil {
		// host 已在 deriveGitHost 校验过，这里理论上不会失败；兜底直接拼接。
		return fmt.Sprintf("%s/%s/%s.git", host, owner, repo)
	}
	u.User = url.UserPassword("oauth2", token)
	u.Path = fmt.Sprintf("/%s/%s.git", owner, repo)
	return u.String()
}

// cloneTargetDir 推断克隆后的目标目录：显式指定优先，否则取仓库名。
func cloneTargetDir(dir, repo string) string {
	if dir != "" {
		return dir
	}
	return repo
}
