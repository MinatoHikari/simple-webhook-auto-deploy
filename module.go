package main

import (
	"github.com/kataras/golog"
	"strings"
)

// CheckBranch 分支过滤
func CheckBranch(branch string, reqBodyMap RequestBody, logger *golog.Logger) bool {
	refArr := strings.Split(reqBodyMap.Ref, "/")
	if refArr[2] != branch {
		logger.Info("branch checked failed, stop deploy...")

		return false
	}

	return true
}
