package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kataras/iris/v12"
)

func main() {
	cfgPath := flag.String("c", "config.toml", "配置文件路径")
	flag.Parse()

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}

	mgr, err := NewProjectManager(cfg, *cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(1)
	}

	auth := NewAuth(cfg.Password)

	app := iris.New()
	// iris 自身日志同时打到控制台与系统日志
	app.Logger().SetOutput(io.MultiWriter(os.Stdout, SystemLogWriter{sink: mgr.System()}))

	// webhook:按项目名路由 + 兼容旧的单项目裸路径
	app.Post("/webhook/{name}", func(ctx iris.Context) {
		p := mgr.Get(ctx.Params().Get("name"))
		if p == nil {
			ctx.StatusCode(http.StatusNotFound)
			_, _ = ctx.WriteString("unknown project")
			return
		}
		handleWebhook(ctx, mgr, p)
	})
	app.Post("/webhook", func(ctx iris.Context) {
		list := mgr.List()
		if len(list) != 1 {
			ctx.StatusCode(http.StatusBadRequest)
			_, _ = ctx.WriteString("multiple projects, please use /webhook/{name}")
			return
		}
		handleWebhook(ctx, mgr, list[0])
	})

	RegisterApi(app, mgr, auth)

	// 提前构建路由器,使 app 可直接作为 http.Handler 使用
	if err := app.Build(); err != nil {
		fmt.Fprintln(os.Stderr, "构建路由失败:", err)
		os.Exit(1)
	}

	// 根处理器:/api 与 /webhook 交给 iris,其余全部服务前端 SPA
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/webhook") {
			app.ServeHTTP(w, r)
			return
		}
		serveSPA(w, r)
	})

	fmt.Printf("webhook-service 已启动: http://%s(监听 %s,项目数: %d)\n",
		cfg.ListenDisplayAddr(), cfg.ListenAddr(), len(mgr.List()))
	if !auth.Enabled() {
		fmt.Println("警告: 未配置管理密码(password),Web 界面无需登录")
	}
	if err := http.ListenAndServe(cfg.ListenAddr(), root); err != nil {
		fmt.Fprintln(os.Stderr, "服务启动失败:", err)
		os.Exit(1)
	}
}

// handleWebhook 校验 webhook 请求并触发对应项目的部署。
func handleWebhook(ctx iris.Context, mgr *ProjectManager, p *Project) {
	var hook Hook
	if err := ctx.ReadHeaders(&hook); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		_, _ = ctx.WriteString("read headers failed")
		return
	}

	reqBody, err := ctx.GetBody()
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		_, _ = ctx.WriteString("read body failed")
		return
	}

	token := p.Cfg.Token
	if token == "" {
		token = mgr.GlobalToken()
	}
	if !Verify(hook, token, reqBody) {
		p.Sink.Log("warn", "webhook", "", "webhook 认证失败,已拒绝")
		ctx.StatusCode(http.StatusUnauthorized)
		_, _ = ctx.WriteString("Unauthorized")
		return
	}

	var body RequestBody
	_ = json.Unmarshal(reqBody, &body) // 自定义触发可能没有 JSON 体,忽略解析错误
	branch, ok := BranchMatches(p.Cfg.Branch, body)
	if !ok {
		p.Sink.Log("info", "webhook", "", "分支不匹配,跳过部署(期望 "+p.Cfg.Branch+", 实际 "+branch+")")
		_, _ = ctx.WriteString("branch mismatch, deploy skipped")
		return
	}

	res := p.EnqueueDeploy("webhook")
	p.Sink.Log("info", "webhook", "", "webhook 触发部署(分支: "+branch+")")
	_, _ = ctx.WriteString("accepted: " + res)
}

// SystemLogWriter 把 iris/golog 的文本日志按行转入系统 LogSink。
type SystemLogWriter struct {
	sink *LogSink
}

func (w SystemLogWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		level := "info"
		l := strings.ToLower(line)
		if strings.Contains(l, "[erro") {
			level = "error"
		} else if strings.Contains(l, "[warn") {
			level = "warn"
		}
		w.sink.Log(level, "server", "", line)
	}
	return len(p), nil
}
