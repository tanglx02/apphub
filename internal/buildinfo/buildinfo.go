// Package buildinfo 保存编译期注入的版本信息。
package buildinfo

import (
	"fmt"
	"runtime"
)

// 由 build.sh 通过 -ldflags -X 注入
var (
	Version   = "1.0.0"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

// APIVersion 为对外 REST API 版本。
const APIVersion = "v1"

// String 返回人类可读的版本信息。
func String() string {
	return fmt.Sprintf("AppHub v%s\nCommit: %s\nBuild: %s\nGo: %s %s/%s",
		Version, GitCommit, BuildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// Short 返回简短版本号。
func Short() string {
	return "v" + Version
}
