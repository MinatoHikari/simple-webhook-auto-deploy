package main

import (
	"github.com/kataras/iris/v12"
	"os"
)

// InitLog 初始化日志
func InitLog(app *iris.Application) {
	env := GetEnv()

	if env.LogPath == "" {
		env.LogPath = "webhook.log"
	}

	file, _ := os.OpenFile(env.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	logger := app.Logger().AddOutput(file)
	logger.Info("log output:", env.LogPath)
}
