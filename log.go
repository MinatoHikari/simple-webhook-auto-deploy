package main

import (
	"encoding/json"
	"fmt"
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
	depIDs     map[string]bool // 仍拥有日志条目的部署会话集合(懒构建缓存)
	depIDsInit bool
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
	// 会话集合缓存命中时增量维护
	if s.depIDsInit && e.DeploymentID != "" {
		s.depIDs[e.DeploymentID] = true
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

// Delete 硬删除命中条件的日志条目(内存缓冲 + 当前文件与滚动文件),返回删除数量。
// 删除数量以磁盘文件为准(内存条目总是先落盘,避免双重计数)。
func (s *LogSink) Delete(match func(*LogEntry) bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 内存环形缓冲:保留未命中的条目(重新线性化)
	kept := make([]LogEntry, 0, s.ringCount)
	for i := 0; i < s.ringCount; i++ {
		e := s.ring[(s.ringStart+i)%ringSize]
		if !match(&e) {
			kept = append(kept, e)
		}
	}
	s.ring, s.ringStart, s.ringCount = kept, 0, len(kept)

	// 磁盘:重写当前文件与滚动文件,丢弃命中行(解析失败的行保留)
	deleted := 0
	for _, p := range []string{s.path + ".1", s.path} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue // 文件不存在则跳过
		}
		var b strings.Builder
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			var e LogEntry
			if json.Unmarshal([]byte(line), &e) == nil && match(&e) {
				deleted++
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if p == s.path && s.file != nil {
			// 重命名前先关闭追加句柄,写完重新打开
			_ = s.file.Close()
			s.file = nil
		}
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, []byte(b.String()), 0644); err != nil {
			return deleted, fmt.Errorf("重写日志文件失败: %w", err)
		}
		if err := os.Rename(tmp, p); err != nil {
			return deleted, fmt.Errorf("替换日志文件失败: %w", err)
		}
		if p == s.path {
			f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return deleted, fmt.Errorf("重新打开日志文件失败: %w", err)
			}
			s.file = f
			if fi, err := f.Stat(); err == nil {
				s.size = fi.Size()
			}
		}
	}

	s.depIDsInit = false // 会话集合失效,下次查询重建
	return deleted, nil
}

// DeleteAll 清空全部日志(当前文件、滚动文件与内存缓冲),返回删除的条目数(按非空行近似)。
func (s *LogSink) DeleteAll() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, p := range []string{s.path + ".1", s.path} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				n++
			}
		}
	}

	if s.file != nil {
		_ = s.file.Close()
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		s.file = nil
		return n, fmt.Errorf("清空日志文件失败: %w", err)
	}
	s.file = f
	s.size = 0
	_ = os.Remove(s.path + ".1")

	s.ring = s.ring[:0]
	s.ringStart = 0
	s.ringCount = 0

	s.depIDsInit = false
	s.depIDs = nil
	return n, nil
}

// DeploymentIDs 返回仍然拥有日志条目的部署会话 ID 集合(懒构建缓存:
// 首次从内存与磁盘构建,之后由 Log 增量维护,删除后失效重建)。
func (s *LogSink) DeploymentIDs() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.depIDsInit {
		return s.depIDs
	}

	ids := map[string]bool{}
	add := func(e LogEntry) {
		if e.DeploymentID != "" {
			ids[e.DeploymentID] = true
		}
	}
	for i := 0; i < s.ringCount; i++ {
		add(s.ring[(s.ringStart+i)%ringSize])
	}
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
			if json.Unmarshal([]byte(line), &e) == nil {
				add(e)
			}
		}
	}
	s.depIDs, s.depIDsInit = ids, true
	return ids
}
