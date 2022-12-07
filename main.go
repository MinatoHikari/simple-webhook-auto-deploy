package main

import (
	"encoding/json"
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

	InitLog(app)

	err := app.Listen(GetPort())
	if err != nil {
		fmt.Println("service start failed:", err)
	}
}

// Hook gitlab webhook.
type Hook struct {
	GitlabToken     string `header:"X-Gitlab-Token"`
	GithubSignature string `header:"X-Hub-Signature-256"`
}

type RequestBody struct {
	Ref string `json:"ref"`
}

// Webhook 接收 gitlab 的 webhook 请求.
func Webhook(ctx iris.Context) {
	var hook Hook

	var reqBodyMap RequestBody

	logger := ctx.Application().Logger()

	err := ctx.ReadHeaders(&hook)
	if err != nil {
		_, _ = ctx.Writef("read headers failed:", err, "\n")
		logger.Error(err)

		return
	}

	reqBody, err := ctx.GetBody()
	if err != nil {
		logger.Error(err)

		return
	}

	err = json.Unmarshal(reqBody, &reqBodyMap)
	if err != nil {
		logger.Error(err)

		return
	}

	env := GetEnv()

	if Verify(hook, *Token, reqBody) {
		_, _ = ctx.Writef("auth success\n")

		logger.Info("auth success")
		logger.Info("new process pending...")

		if env.Branch != "" {
			res := CheckBranch(env.Branch, reqBodyMap, logger)
			if !res {
				return
			}
		}

		select {
		case Queue <- 1:
			logger.Info("enter into process")

			go RunDeployProcess(logger, env)
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
func RunDeployProcess(logger *golog.Logger, env *ConfigEnv) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("recover error:", err)
		}
	}()

	logger.Info("clean untracked files...")

	if err := GitClean(logger); err != nil {
		logger.Error("git clean failed:", err)

		<-Queue

		Checkqueue(logger, env)

		return
	}

	logger.Info("start pull...")

	if err := GitPull(logger); err != nil {
		logger.Error("git pull failed:", err)

		<-Queue

		Checkqueue(logger, env)

		return
	}

	logger.Info("start build...")

	if err := RunBuild(logger, env.Script, env.PackageManager); err != nil {
		logger.Error("build failed:", err)

		<-Queue

		Checkqueue(logger, env)

		return
	}

	if err := RunAdditionScript(logger, env); err != nil {
		logger.Error("run additional script failed:", err)

		<-Queue

		Checkqueue(logger, env)

		return
	}

	logger.Info("start deploy...")

	if err := ClearTargetFolderThenMove(logger, env.Path, env.Dist); err != nil && env.Path != "" && env.Dist != "" {
		logger.Error("clear failed:", err)

		<-Queue

		Checkqueue(logger, env)

		return
	}

	logger.Info("process end")

	<-Queue

	Checkqueue(logger, env)
}

// Checkqueue check if there are other processes exist
func Checkqueue(logger *golog.Logger, env *ConfigEnv) {
	if len(Processes) != 0 {
		Queue <- 1

		logger.Info("start another deploy process...")

		go RunDeployProcess(logger, env)

		Processes = Processes[1:]
	}
}
