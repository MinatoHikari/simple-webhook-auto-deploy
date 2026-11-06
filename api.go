package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kataras/iris/v12"
)

const (
	sessionCookie = "ws_session"
	sessionTTL    = 7 * 24 * time.Hour
)

// Auth 简单密码登录:内存会话表 + HttpOnly cookie。
// password 为空时整个管理界面免认证。
type Auth struct {
	password string
	mu       sync.Mutex
	sessions map[string]time.Time
}

// NewAuth 创建认证器。
func NewAuth(password string) *Auth {
	return &Auth{password: password, sessions: map[string]time.Time{}}
}

// Enabled 是否启用了密码认证。
func (a *Auth) Enabled() bool { return a.password != "" }

// Login 校验密码,成功返回会话 ID。
func (a *Auth) Login(password string) (string, bool) {
	if a.password == "" {
		return "", true // 免认证模式
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) != 1 {
		return "", false
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	sid := hex.EncodeToString(b)
	a.mu.Lock()
	a.sessions[sid] = time.Now().Add(sessionTTL)
	for k, exp := range a.sessions { // 顺手清理过期会话
		if time.Now().After(exp) {
			delete(a.sessions, k)
		}
	}
	a.mu.Unlock()
	return sid, true
}

// Valid 校验会话是否有效。
func (a *Auth) Valid(sid string) bool {
	if a.password == "" {
		return true
	}
	if sid == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[sid]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(a.sessions, sid)
		return false
	}
	return true
}

// Logout 注销会话。
func (a *Auth) Logout(sid string) {
	a.mu.Lock()
	delete(a.sessions, sid)
	a.mu.Unlock()
}

// Api REST 接口集合。
type Api struct {
	mgr  *ProjectManager
	auth *Auth
}

// RegisterApi 注册全部 /api 路由。
func RegisterApi(app *iris.Application, mgr *ProjectManager, auth *Auth) {
	api := &Api{mgr: mgr, auth: auth}

	// 登录相关不需要认证
	app.Post("/api/login", api.handleLogin)
	app.Post("/api/logout", api.handleLogout)
	app.Get("/api/auth", api.handleAuthCheck)

	p := app.Party("/api")
	p.Use(api.authMiddleware)
	p.Get("/projects", api.listProjects)
	p.Post("/projects", api.createProject)
	p.Get("/projects/{name}", api.getProject)
	p.Put("/projects/{name}", api.updateProject)
	p.Delete("/projects/{name}", api.deleteProject)
	p.Post("/projects/{name}/deploy", api.deployProject)
	p.Post("/projects/{name}/branch", api.switchBranch)
	p.Get("/projects/{name}/logs", api.projectLogs)
	p.Get("/projects/{name}/logs/stream", api.projectLogStream)
	p.Get("/logs", api.systemLogs)
	p.Get("/logs/stream", api.globalLogStream)
}

func jsonError(ctx iris.Context, code int, msg string) {
	ctx.StatusCode(code)
	_, _ = ctx.JSON(map[string]string{"error": msg})
}

func (api *Api) authMiddleware(ctx iris.Context) {
	if !api.auth.Enabled() || api.auth.Valid(ctx.GetCookie(sessionCookie)) {
		ctx.Next()
		return
	}
	jsonError(ctx, http.StatusUnauthorized, "未登录或会话已过期")
}

type loginRequest struct {
	Password string `json:"password"`
}

func (api *Api) handleLogin(ctx iris.Context) {
	var req loginRequest
	if err := ctx.ReadJSON(&req); err != nil {
		jsonError(ctx, http.StatusBadRequest, "请求体格式错误")
		return
	}
	sid, ok := api.auth.Login(req.Password)
	if !ok {
		api.mgr.System().Log("warn", "server", "", "Web 界面登录失败(密码错误)")
		jsonError(ctx, http.StatusUnauthorized, "密码错误")
		return
	}
	if sid != "" {
		http.SetCookie(ctx.ResponseWriter(), &http.Cookie{
			Name:     sessionCookie,
			Value:    sid,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(sessionTTL.Seconds()),
		})
	}
	api.mgr.System().Log("info", "server", "", "Web 界面登录成功")
	_, _ = ctx.JSON(map[string]bool{"ok": true})
}

func (api *Api) handleLogout(ctx iris.Context) {
	api.auth.Logout(ctx.GetCookie(sessionCookie))
	http.SetCookie(ctx.ResponseWriter(), &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	_, _ = ctx.JSON(map[string]bool{"ok": true})
}

func (api *Api) handleAuthCheck(ctx iris.Context) {
	_, _ = ctx.JSON(map[string]interface{}{
		"authed":  api.auth.Valid(ctx.GetCookie(sessionCookie)),
		"enabled": api.auth.Enabled(),
	})
}

func (api *Api) listProjects(ctx iris.Context) {
	list := api.mgr.List()
	out := make([]ProjectStatus, 0, len(list))
	for _, p := range list {
		out = append(out, p.Snapshot())
	}
	_, _ = ctx.JSON(out)
}

// projectDetail 详情响应:状态 + 完整配置。
type projectDetail struct {
	Status ProjectStatus `json:"status"`
	Cfg    ProjectConfig `json:"cfg"`
}

func newDetail(p *Project) projectDetail {
	return projectDetail{Status: p.Snapshot(), Cfg: p.Cfg}
}

func (api *Api) getProject(ctx iris.Context) {
	p := api.mgr.Get(ctx.Params().Get("name"))
	if p == nil {
		jsonError(ctx, http.StatusNotFound, "项目不存在")
		return
	}
	_, _ = ctx.JSON(newDetail(p))
}

func (api *Api) createProject(ctx iris.Context) {
	var cfg ProjectConfig
	if err := ctx.ReadJSON(&cfg); err != nil {
		jsonError(ctx, http.StatusBadRequest, "请求体格式错误")
		return
	}
	p, err := api.mgr.CreateProject(cfg)
	if err != nil {
		jsonError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.StatusCode(http.StatusCreated)
	_, _ = ctx.JSON(newDetail(p))
}

func (api *Api) updateProject(ctx iris.Context) {
	name := ctx.Params().Get("name")
	var cfg ProjectConfig
	if err := ctx.ReadJSON(&cfg); err != nil {
		jsonError(ctx, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := api.mgr.UpdateProject(name, cfg); err != nil {
		jsonError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = ctx.JSON(newDetail(api.mgr.Get(name)))
}

func (api *Api) deleteProject(ctx iris.Context) {
	name := ctx.Params().Get("name")
	if err := api.mgr.DeleteProject(name); err != nil {
		jsonError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = ctx.JSON(map[string]bool{"ok": true})
}

func (api *Api) deployProject(ctx iris.Context) {
	p := api.mgr.Get(ctx.Params().Get("name"))
	if p == nil {
		jsonError(ctx, http.StatusNotFound, "项目不存在")
		return
	}
	res := p.EnqueueDeploy("manual")
	_, _ = ctx.JSON(map[string]string{"result": res})
}

type branchRequest struct {
	Branch string `json:"branch"`
}

func (api *Api) switchBranch(ctx iris.Context) {
	p := api.mgr.Get(ctx.Params().Get("name"))
	if p == nil {
		jsonError(ctx, http.StatusNotFound, "项目不存在")
		return
	}
	var req branchRequest
	if err := ctx.ReadJSON(&req); err != nil || req.Branch == "" {
		jsonError(ctx, http.StatusBadRequest, "分支名不能为空")
		return
	}
	if p.Status() == StatusDeploying {
		jsonError(ctx, http.StatusConflict, "部署进行中,无法切换分支")
		return
	}
	if err := p.SwitchBranch(req.Branch); err != nil {
		jsonError(ctx, http.StatusInternalServerError, "切换分支失败: "+err.Error())
		return
	}
	if err := api.mgr.PersistConfig(); err != nil {
		jsonError(ctx, http.StatusInternalServerError, "配置写回失败: "+err.Error())
		return
	}
	p.Sink.Log("info", "checkout", "", "分支已切换为 "+req.Branch)
	_, _ = ctx.JSON(newDetail(p))
}

// parseLogFilter 从查询参数解析日志筛选条件。
// level=info,error  q=关键词  deployment=会话ID  since/until=RFC3339  limit=条数
func parseLogFilter(ctx iris.Context) (LogFilter, int) {
	f := LogFilter{}
	if lv := ctx.URLParam("level"); lv != "" {
		f.Levels = map[string]bool{}
		for _, s := range strings.Split(lv, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			if s == "info" || s == "warn" || s == "error" {
				f.Levels[s] = true
			}
		}
	}
	f.Query = ctx.URLParam("q")
	f.Deployment = ctx.URLParam("deployment")
	if s := ctx.URLParam("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Since = t
		}
	}
	if s := ctx.URLParam("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Until = t
		}
	}
	limit := 500
	if s := ctx.URLParam("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 5000 {
		limit = 5000
	}
	return f, limit
}

func (api *Api) projectLogs(ctx iris.Context) {
	p := api.mgr.Get(ctx.Params().Get("name"))
	if p == nil {
		jsonError(ctx, http.StatusNotFound, "项目不存在")
		return
	}
	filter, limit := parseLogFilter(ctx)
	entries := p.Sink.History(filter, limit)
	if entries == nil {
		entries = []LogEntry{}
	}
	_, _ = ctx.JSON(map[string]interface{}{"entries": entries})
}

func (api *Api) systemLogs(ctx iris.Context) {
	filter, limit := parseLogFilter(ctx)
	entries := api.mgr.System().History(filter, limit)
	if entries == nil {
		entries = []LogEntry{}
	}
	_, _ = ctx.JSON(map[string]interface{}{"entries": entries})
}

// serveLogStream 通用的 SSE 日志流:按筛选条件推送实时日志。
func serveLogStream(ctx iris.Context, hub *LogHub, filter LogFilter) {
	ctx.ContentType("text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	ctx.WriteString("retry: 3000\n\n")
	ctx.WriteString("data: " + `{"type":"connected"}` + "\n\n")
	ctx.ResponseWriter().Flush()

	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return
			}
			if filter.Match(&e) {
				if b, err := json.Marshal(e); err == nil {
					ctx.WriteString("data: " + string(b) + "\n\n")
					ctx.ResponseWriter().Flush()
				}
			}
		case <-ticker.C:
			ctx.WriteString(": heartbeat\n\n")
			ctx.ResponseWriter().Flush()
		case <-ctx.Request().Context().Done():
			return
		}
	}
}

func (api *Api) projectLogStream(ctx iris.Context) {
	p := api.mgr.Get(ctx.Params().Get("name"))
	if p == nil {
		jsonError(ctx, http.StatusNotFound, "项目不存在")
		return
	}
	filter, _ := parseLogFilter(ctx)
	serveLogStream(ctx, p.Sink.Hub(), filter)
}

func (api *Api) globalLogStream(ctx iris.Context) {
	filter, _ := parseLogFilter(ctx)
	serveLogStream(ctx, api.mgr.GlobalHub(), filter)
}
