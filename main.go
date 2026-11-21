package main

import (
	"os"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/cliapp"
)

var (
	// Version 版本号
	Version = "v5.0.1-dev"
)

func main() {
	if err := cliapp.Run(Version, os.Args); err != nil {
		os.Exit(1)
	}
}
