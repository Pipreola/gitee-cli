package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"gitee-cli/pkg/auth"
	"gitee-cli/pkg/config"
)

// newAuthCmd 返回认证相关的命令组：login / status / logout。
func newAuthCmd() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "管理 Gitee 认证凭证",
	}
	authCmd.AddCommand(newAuthLoginCmd())
	authCmd.AddCommand(newAuthStatusCmd())
	authCmd.AddCommand(newAuthLogoutCmd())
	return authCmd
}

// newAuthLoginCmd 返回使用私人令牌登录的命令。
func newAuthLoginCmd() *cobra.Command {
	var token string
	var host string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "使用私人令牌登录 Gitee",
		Long: `通过 Gitee 私人令牌（personal access token）登录，校验通过后凭证将保存到本地配置文件。

如何获取私人令牌：
  访问 https://gitee.com/profile/personal_access_tokens 生成新令牌。
  建议勾选 user_info、projects、pull_requests、issues 等权限。

使用方式：
  1. 通过 --token 参数直接传入：
     gitee auth login --token <your-token>

  2. 交互式输入（推荐）：
     gitee auth login
     然后按提示粘贴令牌（输入不会显示，但会被正确读取）`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 如果未通过 --token 参数指定，则交互式读取
			if token == "" {
				fmt.Fprint(cmd.OutOrStdout(), "请粘贴你的 Gitee 私人令牌（输入不会显示）：")
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "获取令牌: https://gitee.com/profile/personal_access_tokens")

				// 使用 term.ReadPassword 实现无回显输入
				// 尝试从 stdin 文件描述符读取，如果失败（非 TTY 场景）则返回明确错误
				input, err := term.ReadPassword(int(os.Stdin.Fd()))
				if err != nil {
					return fmt.Errorf("读取密码失败（请确认在终端中运行或使用 --token 参数）: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "") // 输入后换行

				token = strings.TrimSpace(string(input))
				if token == "" {
					return fmt.Errorf("令牌不能为空")
				}
			}

			user, err := auth.Login(cmd.Context(), host, token)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "✓ 登录成功，当前用户: %s (%s)\n", user.Name, user.Login)
			return err
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "Gitee 私人令牌（可选，留空则交互式输入）")
	cmd.Flags().StringVar(&host, "host", config.DefaultHost, "Gitee API 基础地址")
	return cmd
}

// newAuthStatusCmd 返回查看当前登录状态的命令。
func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看当前登录状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			user, loggedIn, err := auth.Status(cmd.Context())
			if err != nil {
				return err
			}
			if !loggedIn {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "✗ 未登录")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "使用 'gitee auth login' 命令登录")
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "✓ 已登录\n用户: %s\n主页: %s\n", user.Login, user.HTMLURL)
			return err
		},
	}
}

// newAuthLogoutCmd 返回登出命令，清除本地凭证。
func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "登出并清除本地凭证",
		Long:  "删除保存在本地配置文件中的 Gitee 认证令牌，登出后需要重新运行 'gitee auth login' 才能使用需要认证的功能。",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.Logout(); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "✓ 已登出，本地凭证已清除")
			return err
		},
	}
}
