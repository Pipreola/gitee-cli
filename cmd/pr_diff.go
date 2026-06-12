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

type prDiffOptions struct {
	number   int64
	nameOnly bool
	color    string
}

type prDiffEnv struct {
	git        gitRunner
	loadConfig func() (*config.Config, error)
	getDiff    func(ctx context.Context, host, token, owner, repo string, number int64) (string, error)
	out        io.Writer
	isTTY      func() bool
}

func defaultPRDiffEnv() prDiffEnv {
	return prDiffEnv{
		git:        execGitRunner{},
		loadConfig: config.Load,
		getDiff: func(ctx context.Context, host, token, owner, repo string, number int64) (string, error) {
			client := api.NewClient(host, token)
			return client.GetPullRequestDiff(ctx, owner, repo, number)
		},
		out:   os.Stdout,
		isTTY: stdoutIsTTY,
	}
}

func newPRDiffCmd() *cobra.Command {
	opts := prDiffOptions{}

	cmd := &cobra.Command{
		Use:   "diff <number>",
		Short: "查看 Pull Request 的代码差异",
		Long: `查看指定编号 Pull Request 的 unified diff。

默认输出完整 diff 文本。使用 --name-only 仅列出变更文件名，
使用 --color 控制着色（auto/always/never）。`,
		Example: `  # 查看 PR #123 的 diff
  gitee pr diff 123

  # 仅列出变更文件名
  gitee pr diff 123 --name-only

  # 强制着色输出
  gitee pr diff 123 --color always`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("无效的 PR 编号 %q，必须是正整数", args[0])
			}
			opts.number = n
			return runPRDiff(context.Background(), opts, defaultPRDiffEnv())
		},
	}

	cmd.Flags().BoolVar(&opts.nameOnly, "name-only", false, "仅输出变更的文件名")
	cmd.Flags().StringVar(&opts.color, "color", "auto", "着色模式: auto/always/never")

	return cmd
}

func runPRDiff(ctx context.Context, opts prDiffOptions, env prDiffEnv) error {
	cfg, err := env.loadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("未登录，请先运行 'gitee auth login' 进行认证")
	}

	owner, repo, err := getCurrentRepo(env.git)
	if err != nil {
		return fmt.Errorf("获取仓库信息失败: %w", err)
	}

	diff, err := env.getDiff(ctx, cfg.Host, cfg.Token, owner, repo, opts.number)
	if err != nil {
		return fmt.Errorf("获取 PR diff 失败: %w", err)
	}

	if opts.nameOnly {
		names := extractFileNames(diff)
		for _, name := range names {
			fmt.Fprintln(env.out, name)
		}
		return nil
	}

	useColor := resolveColor(opts.color, env.isTTY())
	if useColor {
		writeDiffColored(env.out, diff)
	} else {
		fmt.Fprint(env.out, diff)
	}
	return nil
}

func extractFileNames(diff string) []string {
	seen := make(map[string]bool)
	var names []string
	lines := strings.Split(diff, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			// Modified or added file
			name := strings.TrimPrefix(line, "+++ b/")
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		} else if strings.HasPrefix(line, "+++ /dev/null") {
			// Deleted file: extract from preceding --- a/ line
			if i > 0 && strings.HasPrefix(lines[i-1], "--- a/") {
				name := strings.TrimPrefix(lines[i-1], "--- a/")
				if name != "" && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func resolveColor(mode string, isTTY bool) bool {
	switch strings.ToLower(mode) {
	case "always":
		return true
	case "never":
		return false
	default:
		return isTTY
	}
}

func writeDiffColored(w io.Writer, diff string) {
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			fmt.Fprintln(w, colorCyan+line+colorReset)
		case strings.HasPrefix(line, "@@"):
			fmt.Fprintln(w, colorMagenta+line+colorReset)
		case strings.HasPrefix(line, "+"):
			fmt.Fprintln(w, colorGreen+line+colorReset)
		case strings.HasPrefix(line, "-"):
			fmt.Fprintln(w, colorRed+line+colorReset)
		case strings.HasPrefix(line, "diff --git"):
			fmt.Fprintln(w, colorCyan+line+colorReset)
		default:
			fmt.Fprintln(w, line)
		}
	}
}
