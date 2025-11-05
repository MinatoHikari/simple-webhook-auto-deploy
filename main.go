package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

var Token *string
var Queue chan int
var Processes []int
var LogChannel chan string // 用于实时日志推送的 channel

func main() {
	notFound := InitConfig()
	if notFound {
		Token = flag.String("t", "", "gitlab 设置的 X-Gitlab-Token")
		flag.Parse()
	} else {
		Token = GetToken()
	}

	Queue = make(chan int, 1)
	LogChannel = make(chan string, 100) // 初始化日志 channel

	app := iris.New()
	app.Post("/webhook", Webhook)
	app.Get("/log", LogPage)
	app.Get("/log/stream", LogStream)

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
	Ref    string `json:"ref"`
	Branch string `json:"branch"`
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
		_, _ = ctx.Writef("Auth success, a new script has been pushed into task list.")

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

	if env.AdditionalScript != "" {
		if err := RunAdditionScript(logger, env); err != nil {
			logger.Error("run additional script failed:", err)

			<-Queue

			Checkqueue(logger, env)

			return
		}
	}

	if err := RunBuild(logger, env.Script, env.PackageManager); err != nil {
		logger.Error("build failed:", err)

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

// LogStream 提供 SSE 流来推送实时日志
func LogStream(ctx iris.Context) {
	ctx.ContentType("text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")

	// 发送初始连接消息
	ctx.WriteString("data: " + `{"message":"日志流已连接","level":"info"}` + "\n\n")
	ctx.ResponseWriter().Flush()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	// 缓冲区用于合并多行日志
	var messageBuffer []string
	var bufferTimestamp string
	var bufferLevel string
	var isNewLogEntry bool
	var lastLineWasEmpty bool // 跟踪上一行是否为空行

	for {
		select {
		case logMsg := <-LogChannel:
			// 检查是否为空行（只包含空白字符）
			trimmed := strings.TrimSpace(logMsg)
			
			// 过滤掉常见的文件关闭错误消息，避免显示在网页上
			if strings.Contains(trimmed, "file already closed") ||
			   strings.Contains(trimmed, "read |0: file already closed") ||
			   strings.Contains(trimmed, "broken pipe") {
				continue
			}
			
			isEmpty := trimmed == ""
			
			// 如果连续两行都为空行，表示新的日志条目开始
			if isEmpty && lastLineWasEmpty {
				// 先发送缓冲区中的内容（如果有）
				if len(messageBuffer) > 0 {
					mergedMsg := strings.Join(messageBuffer, " ")
					mergedMsg = strings.Join(strings.Fields(mergedMsg), " ")
					
					if strings.TrimSpace(mergedMsg) != "" {
						timestamp := bufferTimestamp
						if timestamp == "" {
							timestamp = time.Now().Format("15:04:05")
						}
						level := bufferLevel
						if level == "" {
							level = "info"
						}
						
						data := fmt.Sprintf(`{"message":%q,"level":%q,"timestamp":%q}`, mergedMsg, level, timestamp)
						ctx.WriteString("data: " + data + "\n\n")
						ctx.ResponseWriter().Flush()
					}
					
					// 清空缓冲区
					messageBuffer = nil
					bufferTimestamp = ""
					bufferLevel = ""
				}
				
				// 开始新的日志条目
				isNewLogEntry = true
				lastLineWasEmpty = false
				continue
			}
			
			// 更新空行状态
			lastLineWasEmpty = isEmpty
			
			// 如果是空行，跳过
			if isEmpty {
				continue
			}
			
			// 如果是新的日志条目开始（第一次遇到非空行）
			if isNewLogEntry {
				// 从当前行提取等级和时间
				if level := extractLevelFromMessage(trimmed); level != "" {
					bufferLevel = level
					// 移除等级标记
					trimmed = removeLevelFromMessage(trimmed)
				} else {
					// 如果没有找到等级标记，默认为 info
					bufferLevel = "info"
				}
				
				if timePart := extractTimeFromMessage(trimmed); timePart != "" {
					bufferTimestamp = timePart
				}
				
				// 添加到缓冲区
				if strings.TrimSpace(trimmed) != "" {
					messageBuffer = append(messageBuffer, trimmed)
				}
				isNewLogEntry = false
				continue
			}
			
			// 继续添加到缓冲区
			// 尝试从当前行提取等级（可能在某些行中）
			if level := extractLevelFromMessage(trimmed); level != "" {
				bufferLevel = level
				// 移除等级标记
				trimmed = removeLevelFromMessage(trimmed)
			}
			// 尝试从当前行提取时间
			if timePart := extractTimeFromMessage(trimmed); timePart != "" {
				bufferTimestamp = timePart
			}
			
			if strings.TrimSpace(trimmed) != "" {
				messageBuffer = append(messageBuffer, trimmed)
			}
		case <-ticker.C:
			// 如果缓冲区有内容，尝试发送（避免内容滞留）
			if len(messageBuffer) > 0 {
				mergedMsg := strings.Join(messageBuffer, " ")
				mergedMsg = strings.Join(strings.Fields(mergedMsg), " ")
				
				if strings.TrimSpace(mergedMsg) != "" {
					timestamp := bufferTimestamp
					if timestamp == "" {
						timestamp = time.Now().Format("15:04:05")
					}
					level := bufferLevel
					if level == "" {
						level = "info"
					}
					
					data := fmt.Sprintf(`{"message":%q,"level":%q,"timestamp":%q}`, mergedMsg, level, timestamp)
					ctx.WriteString("data: " + data + "\n\n")
					ctx.ResponseWriter().Flush()
					
					// 清空缓冲区
					messageBuffer = nil
					bufferTimestamp = ""
					bufferLevel = ""
					isNewLogEntry = false
				}
			} else {
				// 发送心跳保持连接
				ctx.WriteString(": heartbeat\n\n")
				ctx.ResponseWriter().Flush()
			}
		case <-ctx.Request().Context().Done():
			// 客户端断开连接
			return
		}
	}
}
