package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// repoListOptions 收集 repo list 子命令的全部参数。
type repoListOptions struct {
	// org 是要列出仓库的组织（可选）。为空时列出当前用户的仓库。
	org string
	// visibility 是可见性过滤：public / private / all（仅个人仓库接口支持）。
	visibility string
	// sort 是排序字段：created / updated / pushed / full_name。
	sort string
	// direction 是排序方向：asc / desc。
	direction string
	// limit 是返回的最大仓库数量。
	limit int
	// jsonOut 标记是否以 JSON 输出结果。
	jsonOut bool
	// noColor 禁用颜色输出。
	noColor bool
}

// repoListEnv 聚合 repo list 的外部依赖，使核心流程可在测试中完全注入。
type repoListEnv struct {
	loadConfig func() (*config.Config, error)
	listRepos  func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error)
	out        io.Writer
	isTTY      func() bool
	now        func() time.Time
}

// defaultRepoListEnv 返回基于真实配置 / API 的依赖集合。
func defaultRepoListEnv() repoListEnv {
	return repoListEnv{
		loadConfig: config.Load,
		listRepos: func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error) {
			client := api.NewClient(host, token)
			return client.ListRepositories(ctx, org, input)
		},
		out:   os.Stdout,
		isTTY: stdoutIsTTY,
		now:   time.Now,
	}
}

// newRepoListCmd 创建 repo list 子命令。
func newRepoListCmd() *cobra.Command {
	opts := repoListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出仓库",
		Long: `列出当前认证用户或指定组织的仓库。

默认列出当前用户的仓库；使用 --org 列出指定组织的仓库。
使用 --json 输出原始 JSON 便于脚本处理。`,
		Example: `  # 列出当前用户的仓库
  gitee repo list

  # 列出指定组织的仓库
  gitee repo list --org my-org

  # 仅列出私有仓库
  gitee repo list --visibility private

  # 限制数量并按更新时间排序
  gitee repo list --limit 50 --sort updated

  # 输出 JSON
  gitee repo list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoList(context.Background(), opts, defaultRepoListEnv())
		},
	}

	cmd.Flags().StringVarP(&opts.org, "org", "o", "", "列出指定组织的仓库（为空时列出当前用户的仓库）")
	cmd.Flags().StringVar(&opts.visibility, "visibility", "", "可见性过滤：public/private/all（仅个人仓库支持）")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "排序字段：created/updated/pushed/full_name")
	cmd.Flags().StringVar(&opts.direction, "direction", "", "排序方向：asc/desc")
	cmd.Flags().IntVarP(&opts.limit, "limit", "L", 30, "返回的最大仓库数量（1-100）")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "输出 JSON 格式")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "禁用颜色输出")

	return cmd
}

// runRepoList 执行 repo list 的核心流程。
func runRepoList(ctx context.Context, opts repoListOptions, env repoListEnv) error {
	// 1. 校验参数
	if err := validateRepoListOptions(&opts); err != nil {
		return err
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
	input := &api.ListRepositoriesInput{
		Visibility: opts.visibility,
		Sort:       opts.sort,
		Direction:  opts.direction,
		PerPage:    opts.limit,
		Page:       1,
	}

	// 4. 调用 API
	repos, err := env.listRepos(ctx, cfg.Host, cfg.Token, opts.org, input)
	if err != nil {
		return fmt.Errorf("查询仓库列表失败: %w", err)
	}

	// 5. 兜底限制数量
	if opts.limit > 0 && len(repos) > opts.limit {
		repos = repos[:opts.limit]
	}

	// 6. 输出
	if opts.jsonOut {
		return writeJSONValue(env.out, repos)
	}
	useColor := !opts.noColor && env.isTTY()
	return writeRepoList(env.out, repos, opts.org, env.now(), useColor)
}

// validateRepoListOptions 校验 repo list 的输入参数。
func validateRepoListOptions(opts *repoListOptions) error {
	if opts.visibility != "" {
		switch opts.visibility {
		case "public", "private", "all":
		default:
			return fmt.Errorf("无效的 --visibility 值 %q，仅支持 public/private/all", opts.visibility)
		}
	}
	if opts.sort != "" {
		switch opts.sort {
		case "created", "updated", "pushed", "full_name":
		default:
			return fmt.Errorf("无效的 --sort 值 %q，仅支持 created/updated/pushed/full_name", opts.sort)
		}
	}
	if opts.direction != "" {
		switch opts.direction {
		case "asc", "desc":
		default:
			return fmt.Errorf("无效的 --direction 值 %q，仅支持 asc/desc", opts.direction)
		}
	}
	if opts.limit < 1 || opts.limit > 100 {
		return fmt.Errorf("无效的 --limit 值 %d，须在 1-100 之间", opts.limit)
	}
	return nil
}

// writeRepoList 以表格形式输出仓库列表，风格对齐 gh/glab。
func writeRepoList(w io.Writer, repos []api.Repository, org string, now time.Time, useColor bool) error {
	scope := "当前用户"
	if org != "" {
		scope = "组织 " + org
	}

	if len(repos) == 0 {
		_, err := fmt.Fprintf(w, "%s 下没有仓库。\n", scope)
		return err
	}

	fmt.Fprintf(w, "Showing %d %s for %s\n\n", len(repos), pluralize(len(repos), "repository", "repositories"), scope)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tVISIBILITY\tLANGUAGE\tUPDATED")

	for _, r := range repos {
		name := r.FullName
		if name == "" {
			name = r.Name
		}
		visibility := colorize("public", colorGreen, useColor)
		if r.Private {
			visibility = colorize("private", colorYellow, useColor)
		}
		language := r.Language
		if language == "" {
			language = "-"
		}
		// 优先用 pushed_at，回退到 updated_at
		updatedSrc := r.PushedAt
		if updatedSrc == "" {
			updatedSrc = r.UpdatedAt
		}
		updated := relativeTime(updatedSrc, now)

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", colorize(name, colorCyan, useColor), visibility, language, updated)
	}
	return tw.Flush()
}
