package main

import (
	"github.com/kataras/iris/v12"
	"os"
)

// InitLog 初始化日志
func InitLog(app *iris.Application) {
	_, _, _, _, logPath := GetEnv()

	if logPath == "" {
		logPath = "webhook.log"
	}

	file, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	logger := app.Logger().AddOutput(file)
	logger.Info("log output:", logPath)
}
