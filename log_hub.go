package main

import "sync"

// LogHub 日志发布-订阅中枢。
// 每个查看日志的连接拥有独立的订阅 channel,避免多个连接争抢同一个 channel,
// 也避免无人查看时 channel 写满后影响后续连接。
type LogHub struct {
	mu          sync.Mutex
	subscribers map[chan LogEntry]struct{}
}

// NewLogHub 创建日志中枢。
func NewLogHub() *LogHub {
	return &LogHub{
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

// Subscribe 注册一个新的订阅者,返回其专属 channel。
func (h *LogHub) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 256)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe 注销订阅者并关闭其 channel。
func (h *LogHub) Unsubscribe(ch chan LogEntry) {
	h.mu.Lock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Broadcast 向所有订阅者非阻塞地推送一条日志。
// 没有订阅者时直接丢弃,慢消费者满了也直接丢弃,保证写日志永不阻塞。
func (h *LogHub) Broadcast(e LogEntry) {
	h.mu.Lock()
	for ch := range h.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
	h.mu.Unlock()
}
