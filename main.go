package main

import (
	"flag"
	"fmt"
	"github.com/kataras/golog"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

var Token *string
var Queue chan int
var Processes []int

func main() {
	notFound := InitConfig()
	if notFound {
		Token = flag.String("t", "", "gitlab 设置的 X-Gitlab-Token")
		flag.Parse()
	} else {
		Token = GetToken()
	}

	Queue = make(chan int, 1)

	app := iris.New()
	app.Post("/webhook", Webhook)

	err := app.Listen(":8079")
	if err != nil {
		fmt.Println("service start failed:", err)
	}
}

// Hook gitlab webhook.
type Hook struct {
	Authentication string `header:"X-Gitlab-Token,required"`
}

// Webhook 接收 gitlab 的 webhook 请求.
func Webhook(ctx iris.Context) {
	var hook Hook

	logger := ctx.Application().Logger()

	err := ctx.ReadHeaders(&hook)
	if err != nil {
		_, _ = ctx.Writef("read headers failed:", err, "\n")
		logger.Error(err)

		return
	}

	if hook.Authentication == *Token {
		_, _ = ctx.Writef("auth success\n")

		logger.Info("auth success")
		logger.Info("new process pending...")

		select {
		case Queue <- 1:
			logger.Info("enter into process")

			path, script, dist := GetEnv()

			go RunDeployProcess(logger, path, script, dist)
		default:
			Processes = append(Processes, 1)
		}
	} else {
		ctx.StatusCode(httptest.StatusUnauthorized)
		_, _ = ctx.WriteString("Unauthorized")
		logger.Error("Unauthorized")

		return
	}
}

// RunDeployProcess 运行部署进程，进程队列.
func RunDeployProcess(logger *golog.Logger, path string, script string, dist string) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("recover error:", err)
		}
	}()

	logger.Info("clean untracked files...")

	if err := GitClean(logger); err != nil {
		logger.Error("git clean failed:", err)

		<-Queue

		return
	}

	logger.Info("start pull...")

	if err := GitPull(logger); err != nil {
		logger.Error("git pull failed:", err)

		<-Queue

		return
	}

	logger.Info("start build...")

	if err := RunBuild(logger, script); err != nil {
		logger.Error("build failed:", err)

		<-Queue

		return
	}

	logger.Info("start deploy...")

	if err := ClearTargetFolderThenMove(logger, path, dist); err != nil {
		logger.Error("clear failed:", err)

		<-Queue

		return
	}

	logger.Info("successfully deployed")

	<-Queue

	if len(Processes) != 0 {
		Queue <- 1

		logger.Info("start another deploy process...")

		go RunDeployProcess(logger, path, script, dist)

		Processes = Processes[1:]
	}
}
