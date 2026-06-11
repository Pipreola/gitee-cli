// Package cmd 定义 gitee-cli 的命令行命令树，基于 Cobra 框架。
package cmd

import (
	"github.com/spf13/cobra"

	"gitee-cli/internal/version"
)

// NewRootCmd 构造并返回根命令，挂载所有子命令。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "gitee",
		Short: "gitee-cli —— Gitee 命令行工具",
		Long: `gitee-cli 是一个用于与 Gitee（码云）交互的命令行工具，
基于 Gitee OpenAPI v5 实现，提供认证、仓库与用户等操作能力。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(newAuthCmd())
	root.AddCommand(newPRCmd())
	root.AddCommand(newRepoCmd())
	root.AddCommand(newIssueCmd())
	root.AddCommand(newCICmd())
	return root
}
