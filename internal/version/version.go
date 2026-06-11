// Package version 提供 gitee-cli 的版本信息。
// 版本号可在构建时通过 -ldflags 注入。
package version

// Version 是 gitee-cli 的语义化版本号，构建时可通过 ldflags 覆盖。
var Version = "0.1.0-dev"

// Commit 是构建对应的 git 提交哈希，构建时可通过 ldflags 注入。
var Commit = "unknown"

// Date 是构建时间，构建时可通过 ldflags 注入。
var Date = "unknown"

// BuiltBy 是构建者信息，构建时可通过 ldflags 注入。
var BuiltBy = "unknown"
