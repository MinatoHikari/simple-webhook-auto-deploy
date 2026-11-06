package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogEntry 结构化日志条目,序列化为 JSONL 落盘。
type LogEntry struct {
	Time         time.Time `json:"time"`
	Level        string    `json:"level"`              // info | warn | error
	Project      string    `json:"project"`            // 项目名或 "system"
	DeploymentID string    `json:"deployment,omitempty"` // 关联的部署会话
	Stage        string    `json:"stage,omitempty"`    // pull / build / deploy / webhook / server
	Message      string    `json:"message"`
}

const (
	ringSize        = 5000              // 内存环形缓冲条数
	rotateSize int64 = 10 * 1024 * 1024 // 单文件超过 10MB 滚动
)

// LogFilter 日志筛选条件,零值表示不过滤。
type LogFilter struct {
	Levels     map[string]bool // 选中的级别,空 = 全部
	Query      string          // 消息关键词,不区分大小写
	Deployment string          // 指定部署会话
	Since      time.Time       // 起始时间(含)
	Until      time.Time       // 结束时间(含)
}

// Match 判断条目是否通过筛选。
func (f *LogFilter) Match(e *LogEntry) bool {
	if len(f.Levels) > 0 && !f.Levels[e.Level] {
		return false
	}
	if f.Deployment != "" && e.DeploymentID != f.Deployment {
		return false
	}
	if !f.Since.IsZero() && e.Time.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Time.After(f.Until) {
		return false
	}
	if f.Query != "" &&
		!strings.Contains(strings.ToLower(e.Message), strings.ToLower(f.Query)) {
		return false
	}
	return true
}

// LogSink 单个日志目标(某项目或系统日志):
// 写 JSONL 文件(超限滚动)+ 内存环形缓冲 + 实时推送给订阅者。
type LogSink struct {
	mu         sync.Mutex
	project    string
	path       string
	file       *os.File
	size       int64
	ring       []LogEntry
	ringStart  int // 环形缓冲中最早元素的下标
	ringCount  int
	projectHub *LogHub // 本项目专属订阅中枢(系统日志为 nil)
	globalHub  *LogHub // 全局聚合中枢
}

// NewLogSink 创建日志目标并打开日志文件。
func NewLogSink(project, filePath string, projectHub, globalHub *LogHub) (*LogSink, error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	var size int64
	if fi, err := file.Stat(); err == nil {
		size = fi.Size()
	}
	return &LogSink{
		project:    project,
		path:       filePath,
		file:       file,
		size:       size,
		ring:       make([]LogEntry, 0, 1024),
		projectHub: projectHub,
		globalHub:  globalHub,
	}, nil
}

// Log 写入一条日志:落盘 + 进环形缓冲 + 推给订阅者。
func (s *LogSink) Log(level, stage, deployment, msg string) {
	if level == "" {
		level = "info"
	}
	e := LogEntry{
		Time:         time.Now(),
		Level:        level,
		Project:      s.project,
		DeploymentID: deployment,
		Stage:        stage,
		Message:      msg,
	}

	s.mu.Lock()
	if s.file != nil {
		if b, err := json.Marshal(e); err == nil {
			b = append(b, '\n')
			if n, err := s.file.Write(b); err == nil {
				s.size += int64(n)
				if s.size > rotateSize {
					s.rotateLocked()
				}
			}
		}
	}
	if s.ringCount < ringSize {
		s.ring = append(s.ring, e)
		s.ringCount++
	} else {
		s.ring[s.ringStart] = e
		s.ringStart = (s.ringStart + 1) % ringSize
	}
	s.mu.Unlock()

	if s.projectHub != nil {
		s.projectHub.Broadcast(e)
	}
	if s.globalHub != nil && s.globalHub != s.projectHub {
		s.globalHub.Broadcast(e)
	}
}

// rotateLocked 滚动日志文件,需持有锁调用。
func (s *LogSink) rotateLocked() {
	_ = s.file.Close()
	_ = os.Rename(s.path, s.path+".1")
	if f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		s.file = f
		s.size = 0
	} else {
		s.file = nil // 落盘失败不影响内存与推送
	}
}

// Close 关闭日志文件。
func (s *LogSink) Close() {
	s.mu.Lock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	s.mu.Unlock()
}

// Hub 返回本项目专属的订阅中枢(系统日志即全局中枢)。
func (s *LogSink) Hub() *LogHub {
	return s.projectHub
}

// History 返回符合筛选条件的最近 limit 条日志(按时间升序)。
// 优先查内存环形缓冲,不足时回扫磁盘文件补齐。
func (s *LogSink) History(f LogFilter, limit int) []LogEntry {
	if limit <= 0 {
		limit = 500
	}

	s.mu.Lock()
	var matches []LogEntry
	for i := 0; i < s.ringCount; i++ {
		e := s.ring[(s.ringStart+i)%ringSize]
		if f.Match(&e) {
			matches = append(matches, e)
		}
	}
	s.mu.Unlock()
	if len(matches) >= limit {
		return matches[len(matches)-limit:]
	}

	// 内存不足,回扫磁盘(旧滚动文件在前)
	var fromFile []LogEntry
	for _, p := range []string{s.path + ".1", s.path} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			var e LogEntry
			if json.Unmarshal([]byte(line), &e) != nil {
				continue
			}
			if f.Match(&e) {
				fromFile = append(fromFile, e)
			}
		}
	}
	if len(fromFile) > len(matches) {
		matches = fromFile
	}
	if len(matches) > limit {
		matches = matches[len(matches)-limit:]
	}
	return matches
}
