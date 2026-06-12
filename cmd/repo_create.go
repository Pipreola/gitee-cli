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

// repoCreateOptions 收集 repo create 子命令的全部参数。
type repoCreateOptions struct {
	// name 是仓库名称（必填，也可通过位置参数传入）。
	name string
	// description 是仓库描述（可选）。
	description string
	// homepage 是仓库主页地址（可选）。
	homepage string
	// org 是目标组织（可选）。为空时在当前用户名下创建。
	org string
	// private 标记是否创建私有仓库。
	private bool
	// autoInit 标记是否自动初始化仓库（生成 README）。
	autoInit bool
	// web 标记是否在创建后打开浏览器。
	web bool
	// jsonOut 标记是否以 JSON 输出结果。
	jsonOut bool
}

// repoCreateEnv 聚合 repo create 的外部依赖，使核心流程可在测试中完全注入。
type repoCreateEnv struct {
	loadConfig  func() (*config.Config, error)
	createRepo  func(ctx context.Context, host, token, org string, input *api.CreateRepositoryInput) (*api.Repository, error)
	openBrowser func(url string) error
	out         io.Writer
}

// defaultRepoCreateEnv 返回基于真实配置 / API 的依赖集合。
func defaultRepoCreateEnv() repoCreateEnv {
	return repoCreateEnv{
		loadConfig: config.Load,
		createRepo: func(ctx context.Context, host, token, org string, input *api.CreateRepositoryInput) (*api.Repository, error) {
			client := api.NewClient(host, token)
			return client.CreateRepository(ctx, org, input)
		},
		openBrowser: openBrowser,
		out:         os.Stdout,
	}
}

// newRepoCreateCmd 创建 repo create 子命令。
func newRepoCreateCmd() *cobra.Command {
	opts := repoCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "创建仓库",
		Long: `创建一个新的 Gitee 仓库。

仓库名称可通过位置参数或 --name 指定。默认在当前认证用户名下创建；
使用 --org 在指定组织下创建。使用 --private 创建私有仓库。`,
		Example: `  # 在当前用户名下创建公开仓库
  gitee repo create my-repo

  # 创建私有仓库并附带描述
  gitee repo create my-repo --private --description "我的私有项目"

  # 在组织下创建仓库
  gitee repo create my-repo --org my-org

  # 创建并自动初始化（生成 README）
  gitee repo create my-repo --auto-init

  # 创建后在浏览器中打开
  gitee repo create my-repo --web`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.name = args[0]
			}
			return runRepoCreate(context.Background(), opts, defaultRepoCreateEnv())
		},
	}

	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "仓库名称（必填，也可通过位置参数传入）")
	cmd.Flags().StringVarP(&opts.description, "description", "d", "", "仓库描述")
	cmd.Flags().StringVar(&opts.homepage, "homepage", "", "仓库主页地址")
	cmd.Flags().StringVarP(&opts.org, "org", "o", "", "目标组织（为空时在当前用户名下创建）")
	cmd.Flags().BoolVarP(&opts.private, "private", "p", false, "创建为私有仓库")
	cmd.Flags().BoolVar(&opts.autoInit, "auto-init", false, "自动初始化仓库（生成 README）")
	cmd.Flags().BoolVarP(&opts.web, "web", "w", false, "在浏览器中打开创建的仓库")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")

	return cmd
}

// runRepoCreate 执行 repo create 的核心流程。
func runRepoCreate(ctx context.Context, opts repoCreateOptions, env repoCreateEnv) error {
	// 1. 校验仓库名称
	if opts.name == "" {
		return fmt.Errorf("仓库名称不能为空，请通过位置参数或 --name 指定")
	}

	// 2. 加载配置，检查认证
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	// 3. 构造 API 请求
	input := &api.CreateRepositoryInput{
		Name:        opts.name,
		Description: opts.description,
		Homepage:    opts.homepage,
		Private:     opts.private,
		AutoInit:    opts.autoInit,
	}

	// 4. 调用 API 创建仓库
	r, err := env.createRepo(ctx, cfg.Host, cfg.Token, opts.org, input)
	if err != nil {
		return fmt.Errorf("创建仓库失败: %w", err)
	}

	// 5. JSON 输出
	if opts.jsonOut {
		return writeJSONValue(env.out, r)
	}

	// 6. 人类可读输出
	visibility := "public"
	if r.Private {
		visibility = "private"
	}
	name := r.FullName
	if name == "" {
		name = r.Name
	}
	fmt.Fprintf(env.out, "✅ 仓库创建成功！\n")
	fmt.Fprintf(env.out, "   名称: %s (%s)\n", name, visibility)
	if r.Description != "" {
		fmt.Fprintf(env.out, "   描述: %s\n", r.Description)
	}
	fmt.Fprintf(env.out, "   链接: %s\n", r.HTMLURL)

	// 7. 可选：在浏览器中打开
	if opts.web {
		if err := env.openBrowser(r.HTMLURL); err != nil {
			fmt.Fprintf(env.out, "⚠️  无法自动打开浏览器: %v\n", err)
		}
	}

	return nil
}
