package cmd

import "github.com/spf13/cobra"

// newIssueCmd 创建 issue 父命令。
func newIssueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "管理 Issue",
		Long:  "管理 Gitee 仓库的 Issue，包括列表查看、详情查看等功能。",
	}

	cmd.AddCommand(newIssueListCmd())
	cmd.AddCommand(newIssueViewCmd())

	return cmd
}
