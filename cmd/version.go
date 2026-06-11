package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"gitee-cli/internal/version"
)

// newVersionCmd 返回打印版本信息的命令。
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示 gitee-cli 版本信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "gitee-cli %s (commit %s)\n", version.Version, version.Commit)
			return err
		},
	}
}
