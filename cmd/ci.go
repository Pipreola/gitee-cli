// Package cmd 定义 CI 相关命令。
package cmd

import "github.com/spf13/cobra"

// newCICmd 创建 ci 父命令。
func newCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci <command>",
		Short: "管理 CI/CD 状态",
		Long:  "管理 Gitee 仓库的 CI/CD 状态，包括查看构建状态等功能。",
	}

	cmd.AddCommand(newCIStatusCmd())

	return cmd
}
