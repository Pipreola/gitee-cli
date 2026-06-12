package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// repoForkOptions 收集 repo fork 子命令的全部参数。
type repoForkOptions struct {
	// repo 是要 fork 的源仓库定位（owner/repo）。为空时从当前 git remote 推断。
	repo string
	// org 是 fork 到的目标组织（可选）。为空时 fork 到当前用户名下。
	org string
	// name 是 fork 后的新仓库名称（可选）。
	name string
	// web 标记是否在 fork 后打开浏览器。
	web bool
	// jsonOut 标记是否以 JSON 输出结果。
	jsonOut bool
}

// repoForkEnv 聚合 repo fork 的外部依赖，使核心流程可在测试中完全注入。
type repoForkEnv struct {
	git         gitRunner
	loadConfig  func() (*config.Config, error)
	forkRepo    func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error)
	openBrowser func(url string) error
	out         io.Writer
}

// defaultRepoForkEnv 返回基于真实 git / 配置 / API 的依赖集合。
func defaultRepoForkEnv() repoForkEnv {
	return repoForkEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		forkRepo: func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error) {
			client := api.NewClient(host, token)
			return client.ForkRepository(ctx, owner, repo, input)
		},
		openBrowser: openBrowser,
		out:         os.Stdout,
	}
}

// newRepoForkCmd 创建 repo fork 子命令。
func newRepoForkCmd() *cobra.Command {
	opts := repoForkOptions{}

	cmd := &cobra.Command{
		Use:   "fork [owner/repo]",
		Short: "Fork 仓库",
		Long: `Fork 一个 Gitee 仓库到当前用户名下或指定组织。

不指定参数时从当前目录的 git remote 推断源仓库；也可显式传入 owner/repo。
默认 fork 到当前认证用户名下；使用 --org 指定目标组织。`,
		Example: `  # Fork 当前仓库
  gitee repo fork

  # Fork 指定仓库
  gitee repo fork owner/repo

  # Fork 到指定组织
  gitee repo fork owner/repo --org my-org

  # Fork 并重命名
  gitee repo fork owner/repo --name my-fork

  # Fork 后在浏览器中打开
  gitee repo fork owner/repo --web`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.repo = args[0]
			}
			return runRepoFork(context.Background(), opts, defaultRepoForkEnv())
		},
	}

	cmd.Flags().StringVarP(&opts.org, "org", "o", "", "Fork 到的目标组织（为空时 fork 到当前用户名下）")
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Fork 后的新仓库名称")
	cmd.Flags().BoolVarP(&opts.web, "web", "w", false, "在浏览器中打开 fork 后的仓库")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")

	return cmd
}

// runRepoFork 执行 repo fork 的核心流程。
func runRepoFork(ctx context.Context, opts repoForkOptions, env repoForkEnv) error {
	// 1. 加载配置，检查认证
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 2. 确定源仓库 owner/repo：优先使用显式参数，否则从 git remote 推断
	var owner, repo string
	if opts.repo != "" {
		owner, repo, err = parseRepoArg(opts.repo)
		if err != nil {
			return err
		}
	} else {
		owner, repo, err = getCurrentRepo(env.git)
		if err != nil {
			return fmt.Errorf("获取仓库信息失败: %w", err)
		}
	}

	// 3. 构造可选输入：仅当指定了 org 或 name 时才携带 body
	var input *api.ForkRepositoryInput
	if opts.org != "" || opts.name != "" {
		input = &api.ForkRepositoryInput{
			Organization: opts.org,
			Name:         opts.name,
		}
	}

	// 4. 调用 API fork 仓库
	r, err := env.forkRepo(ctx, cfg.Host, cfg.Token, owner, repo, input)
	if err != nil {
		return fmt.Errorf("Fork 仓库失败: %w", err)
	}

	// 5. JSON 输出
	if opts.jsonOut {
		return writeJSONValue(env.out, r)
	}

	// 6. 人类可读输出
	name := r.FullName
	if name == "" {
		name = r.Name
	}
	fmt.Fprintf(env.out, "✅ 已 Fork %s/%s\n", owner, repo)
	fmt.Fprintf(env.out, "   新仓库: %s\n", name)
	fmt.Fprintf(env.out, "   链接: %s\n", r.HTMLURL)

	// 7. 可选：在浏览器中打开
	if opts.web {
		if err := env.openBrowser(r.HTMLURL); err != nil {
			fmt.Fprintf(env.out, "⚠️  无法自动打开浏览器: %v\n", err)
		}
	}

	return nil
}
