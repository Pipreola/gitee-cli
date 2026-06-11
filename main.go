// Command gitee 是 gitee-cli 的程序入口。
package main

import (
	"context"
	"fmt"
	"os"

	"gitee-cli/cmd"
	"gitee-cli/internal/version"
)

// 版本信息，通过 ldflags 在构建时注入
var (
	versionStr = "dev"
	commit     = "none"
	date       = "unknown"
	builtBy    = "unknown"
)

func main() {
	// 设置版本信息
	if versionStr != "" {
		version.Version = versionStr
	}
	if commit != "" {
		version.Commit = commit
	}
	if date != "" {
		version.Date = date
	}
	if builtBy != "" {
		version.BuiltBy = builtBy
	}

	root := cmd.NewRootCmd()
	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
