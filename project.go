package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 项目状态。
const (
	StatusIdle      = "idle"
	StatusQueued    = "queued"
	StatusDeploying = "deploying"
	StatusFailed    = "failed"
)

const maxHistoryLen = 50 // 每个项目保留的部署记录条数

// DeployRecord 一次部署的记录。
type DeployRecord struct {
	ID       string    `json:"id"`
	Trigger  string    `json:"trigger"` // webhook | manual
	Branch   string    `json:"branch,omitempty"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Duration int64     `json:"durationMs"`
	Result   string    `json:"result"` // success | failed
}

// Project 单个项目的运行时:配置、日志、部署队列与状态。
type Project struct {
	Cfg  ProjectConfig
	Sink *LogSink

	mu          sync.Mutex
	queue       chan struct{} // 容量 1,保证同一项目串行部署
	pending     []string      // 排队中的触发来源
	status      string
	currentDep  string
	lastDeploy  time.Time
	lastResult  string
	history     []DeployRecord
	historyPath string
}

// NewProject 创建项目运行时并加载历史部署记录。
func NewProject(cfg ProjectConfig, logDir string, globalHub *LogHub) (*Project, error) {
	hub := NewLogHub()
	sink, err := NewLogSink(cfg.Name, filepath.Join(logDir, cfg.Name+".log"), hub, globalHub)
	if err != nil {
		return nil, err
	}
	p := &Project{
		Cfg:         cfg,
		Sink:        sink,
		queue:       make(chan struct{}, 1),
		status:      StatusIdle,
		historyPath: filepath.Join(logDir, cfg.Name+".history.json"),
	}
	p.loadHistory()
	// 用最近一次部署记录恢复状态摘要,重启后界面仍能显示最近部署时间与结果
	p.mu.Lock()
	if n := len(p.history); n > 0 {
		last := p.history[n-1]
		p.lastDeploy = last.End
		p.lastResult = last.Result
		if last.Result == "failed" {
			p.status = StatusFailed
		}
	}
	p.mu.Unlock()
	return p, nil
}

// ProjectStatus 返回给前端的项目状态视图。
type ProjectStatus struct {
	Name              string           `json:"name"`
	Path              string           `json:"path"`
	Branch            string           `json:"branch"`
	Status            string           `json:"status"`
	CurrentDeployment string           `json:"currentDeployment,omitempty"`
	LastDeploy        time.Time        `json:"lastDeploy"`
	LastResult        string           `json:"lastResult"`
	History           []DeployRecord   `json:"history"`
	LogSessions       map[string]bool  `json:"logSessions"`
}

// Snapshot 生成状态视图(历史记录取最近 20 条)。
func (p *Project) Snapshot() ProjectStatus {
	p.mu.Lock()
	n := len(p.history)
	if n > 20 {
		n = 20
	}
	history := make([]DeployRecord, n)
	copy(history, p.history[len(p.history)-n:])
	if history == nil {
		history = []DeployRecord{}
	}
	st := ProjectStatus{
		Name:              p.Cfg.Name,
		Path:              p.Cfg.Path,
		Branch:            p.Cfg.Branch,
		Status:            p.status,
		CurrentDeployment: p.currentDep,
		LastDeploy:        p.lastDeploy,
		LastResult:        p.lastResult,
		History:           history,
	}
	p.mu.Unlock()

	// 仍拥有日志条目的部署会话(前端据此置灰"查看日志")
	if p.Sink != nil {
		st.LogSessions = p.Sink.DeploymentIDs()
	}
	if st.LogSessions == nil {
		st.LogSessions = map[string]bool{}
	}
	return st
}

// Status 返回当前状态。
func (p *Project) Status() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// EnqueueDeploy 触发一次部署:空闲则立即开始,否则排队。
func (p *Project) EnqueueDeploy(trigger string) string {
	select {
	case p.queue <- struct{}{}:
		go p.deployLoop(trigger)
		return "started"
	default:
		p.mu.Lock()
		p.pending = append(p.pending, trigger)
		if p.status != StatusDeploying {
			p.status = StatusQueued
		}
		p.mu.Unlock()
		return "queued"
	}
}

// deployLoop 依次执行本次及排队中的部署,直到队列清空后释放槽位。
// 槽位在整个循环期间保持占用,避免释放与重新获取之间被新触发者抢到造成重复部署。
func (p *Project) deployLoop(trigger string) {
	for {
		p.runOnce(trigger)

		p.mu.Lock()
		if len(p.pending) == 0 {
			p.mu.Unlock()
			<-p.queue // 释放槽位
			return
		}
		trigger = p.pending[0]
		p.pending = p.pending[1:]
		p.mu.Unlock()
	}
}

// runOnce 执行单次部署并记录结果。
func (p *Project) runOnce(trigger string) {
	depID := newDeploymentID()
	start := time.Now()

	p.mu.Lock()
	p.status = StatusDeploying
	p.currentDep = depID
	// 部署全程使用这份配置快照,持锁读取避免与 UpdateProject 的写入竞态
	cfgSnap := p.Cfg
	p.mu.Unlock()

	p.Sink.Log("info", "deploy", depID, "部署开始(触发来源: "+trigger+")")

	err := p.execute(cfgSnap, depID)

	end := time.Now()
	result := "success"
	level := "info"
	msg := "部署完成,耗时 " + formatDuration(end.Sub(start))
	if err != nil {
		result = "failed"
		level = "error"
		msg = "部署失败(" + err.Error() + "),耗时 " + formatDuration(end.Sub(start))
	}
	p.Sink.Log(level, "deploy", depID, msg)

	rec := DeployRecord{
		ID:       depID,
		Trigger:  trigger,
		Branch:   cfgSnap.Branch,
		Start:    start,
		End:      end,
		Duration: end.Sub(start).Milliseconds(),
		Result:   result,
	}

	p.mu.Lock()
	p.status = map[bool]string{true: StatusIdle, false: StatusFailed}[result == "success"]
	p.currentDep = ""
	p.lastDeploy = end
	p.lastResult = result
	p.appendHistoryLocked(rec)
	historyPath := p.historyPath
	p.mu.Unlock()
	p.saveHistory(historyPath)
}

// appendHistoryLocked 追加部署记录并裁剪,需持有锁调用。
func (p *Project) appendHistoryLocked(rec DeployRecord) {
	p.history = append(p.history, rec)
	if len(p.history) > maxHistoryLen {
		p.history = p.history[len(p.history)-maxHistoryLen:]
	}
}

// loadHistory 从磁盘加载部署历史。
func (p *Project) loadHistory() {
	data, err := os.ReadFile(p.historyPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &p.history)
}

// saveHistory 持久化部署历史(原子写)。
func (p *Project) saveHistory(path string) {
	p.mu.Lock()
	data, err := json.Marshal(p.history)
	p.mu.Unlock()
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// SwitchBranch 切换分支并写回配置。
func (p *Project) SwitchBranch(branch string) error {
	if err := GitCheckout(p.Cfg.Path, branch, p.Sink, ""); err != nil {
		return err
	}
	p.mu.Lock()
	p.Cfg.Branch = branch
	p.mu.Unlock()
	return nil
}

// ProjectManager 管理所有项目的运行时与配置持久化。
type ProjectManager struct {
	mu        sync.RWMutex
	cfg       *AppConfig
	cfgPath   string
	projects  map[string]*Project
	order     []string // 保持配置文件中的项目顺序
	globalHub *LogHub
	system    *LogSink
}

// NewProjectManager 创建管理器并初始化所有项目。
func NewProjectManager(cfg *AppConfig, cfgPath string) (*ProjectManager, error) {
	m := &ProjectManager{
		cfg:       cfg,
		cfgPath:   cfgPath,
		projects:  make(map[string]*Project),
		globalHub: NewLogHub(),
	}
	sysSink, err := NewLogSink("system", filepath.Join(cfg.LogDir, "system.log"), m.globalHub, m.globalHub)
	if err != nil {
		return nil, err
	}
	m.system = sysSink

	for _, pc := range cfg.Projects {
		p, err := NewProject(pc, cfg.LogDir, m.globalHub)
		if err != nil {
			m.system.Log("error", "server", "", "初始化项目 "+pc.Name+" 失败: "+err.Error())
			continue
		}
		m.projects[pc.Name] = p
		m.order = append(m.order, pc.Name)
	}
	return m, nil
}

// Get 按名称查找项目。
func (m *ProjectManager) Get(name string) *Project {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.projects[name]
}

// List 按配置顺序返回全部项目。
func (m *ProjectManager) List() []*Project {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Project, 0, len(m.order))
	for _, name := range m.order {
		if p, ok := m.projects[name]; ok {
			list = append(list, p)
		}
	}
	return list
}

// GlobalToken 返回全局 webhook token。
func (m *ProjectManager) GlobalToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Token
}

// System 返回系统日志 sink。
func (m *ProjectManager) System() *LogSink {
	return m.system
}

// PersistConfig 将当前配置写回磁盘(供分支切换等内部变更使用)。
func (m *ProjectManager) PersistConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return SaveConfig(m.cfg, m.cfgPath)
}

// GlobalHub 返回全局日志中枢。
func (m *ProjectManager) GlobalHub() *LogHub {
	return m.globalHub
}

// CreateProject 新增项目:校验、建运行时、写回配置。
func (m *ProjectManager) CreateProject(pc ProjectConfig) (*Project, error) {
	if err := pc.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[pc.Name]; ok {
		return nil, fmt.Errorf("项目已存在: %s", pc.Name)
	}
	p, err := NewProject(pc, m.cfg.LogDir, m.globalHub)
	if err != nil {
		return nil, err
	}
	m.projects[pc.Name] = p
	m.order = append(m.order, pc.Name)
	m.cfg.Projects = append(m.cfg.Projects, pc)
	if err := SaveConfig(m.cfg, m.cfgPath); err != nil {
		// 回滚运行时
		delete(m.projects, pc.Name)
		m.order = m.order[:len(m.order)-1]
		m.cfg.Projects = m.cfg.Projects[:len(m.cfg.Projects)-1]
		p.Sink.Close()
		return nil, err
	}
	m.system.Log("info", "server", "", "新增项目 "+pc.Name+" ("+pc.Path+")")
	return p, nil
}

// UpdateProject 更新项目配置(不允许改名)。正在部署时禁止修改。
func (m *ProjectManager) UpdateProject(name string, pc ProjectConfig) error {
	if pc.Name != name {
		return fmt.Errorf("不允许修改项目名")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[name]
	if !ok {
		return fmt.Errorf("项目不存在: %s", name)
	}
	if p.Status() == StatusDeploying {
		return fmt.Errorf("部署进行中,请稍后再修改")
	}
	if err := pc.Validate(); err != nil {
		return err
	}
	for i := range m.cfg.Projects {
		if m.cfg.Projects[i].Name == name {
			m.cfg.Projects[i] = pc
			break
		}
	}
	if err := SaveConfig(m.cfg, m.cfgPath); err != nil {
		return err
	}
	p.mu.Lock()
	p.Cfg = pc
	p.mu.Unlock()
	m.system.Log("info", "server", "", "更新项目 "+name+" 配置")
	return nil
}

// DeleteProject 删除项目。正在部署时拒绝。
func (m *ProjectManager) DeleteProject(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[name]
	if !ok {
		return fmt.Errorf("项目不存在: %s", name)
	}
	if p.Status() == StatusDeploying {
		return fmt.Errorf("部署进行中,无法删除")
	}
	delete(m.projects, name)
	for i, n := range m.order {
		if n == name {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	for i := range m.cfg.Projects {
		if m.cfg.Projects[i].Name == name {
			m.cfg.Projects = append(m.cfg.Projects[:i], m.cfg.Projects[i+1:]...)
			break
		}
	}
	if err := SaveConfig(m.cfg, m.cfgPath); err != nil {
		return err
	}
	p.Sink.Close()
	m.system.Log("info", "server", "", "删除项目 "+name)
	return nil
}
