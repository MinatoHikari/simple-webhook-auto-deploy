package main

import (
	"os"
	"regexp"
	"strings"

	"github.com/kataras/iris/v12"
)

// LogWriter 同时写入文件和 channel 的 writer
type LogWriter struct {
	file    *os.File
	channel chan string
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	// 写入文件
	if lw.file != nil {
		lw.file.Write(p)
	}
	// 发送到 channel（非阻塞）
	// 保持原始格式，按行发送（包括空行，以保持换行格式）
	logStr := string(p)
	lines := strings.Split(logStr, "\n")
	for _, line := range lines {
		// 保留原始行内容（包括前导和尾随空格，但不包括换行符）
		// 发送到 channel，空行也发送以保持格式
		select {
		case lw.channel <- line:
		default:
			// channel 满了就跳过，避免阻塞
		}
	}
	return len(p), nil
}

// InitLog 初始化日志
func InitLog(app *iris.Application) {
	env := GetEnv()

	if env.LogPath == "" {
		env.LogPath = "webhook.log"
	}

	file, _ := os.OpenFile(env.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	
	// 创建同时写入文件和 channel 的 writer
	logWriter := &LogWriter{
		file:    file,
		channel: LogChannel,
	}
	
	logger := app.Logger()
	logger.AddOutput(logWriter)
	logger.Info("log output:", env.LogPath)
}

// extractLevelFromMessage 从消息内容中提取日志等级（如 [ERRO], [INFO], [WARN] 等）
func extractLevelFromMessage(message string) string {
	// 匹配 [ERRO], [ERROR], [INFO], [WARN], [WARNING] 等格式
	re := regexp.MustCompile(`(?i)\[?\s*(ERROR|ERRO|INFO|WARN|WARNING|DEBUG)\s*\]?`)
	if matches := re.FindStringSubmatch(message); len(matches) >= 2 {
		levelStr := strings.ToLower(matches[1])
		// 规范化等级名称
		if levelStr == "erro" {
			return "error"
		}
		if levelStr == "warning" {
			return "warn"
		}
		return levelStr
	}
	return ""
}

// removeLevelFromMessage 从消息中移除等级标记
func removeLevelFromMessage(message string) string {
	// 移除 [ERRO], [ERROR], [INFO], [WARN], [WARNING] 等格式
	re := regexp.MustCompile(`(?i)\[?\s*(ERROR|ERRO|INFO|WARN|WARNING|DEBUG)\s*\]?\s*`)
	message = re.ReplaceAllString(message, "")
	// 清理多余空格
	message = strings.Join(strings.Fields(message), " ")
	return strings.TrimSpace(message)
}

// isLogEntryStart 检查是否是日志条目的开始标记（已废弃，改用连续空行作为标记）
// 保留此函数以防其他地方使用，但不再使用
func isLogEntryStart(line string) bool {
	// 此函数已不再使用，保留仅为兼容性
	return false
}

// extractTimeFromMessage 从消息中提取时间戳（HH:MM:SS 格式）
func extractTimeFromMessage(message string) string {
	// 匹配时间格式：2025/11/05 16:49:30 或 2025/11/05 16:49
	re := regexp.MustCompile(`(\d{2}:\d{2}(?::\d{2})?)`)
	if matches := re.FindStringSubmatch(message); len(matches) >= 2 {
		timeStr := matches[1]
		// 如果只有 HH:MM，补全为 HH:MM:SS
		if len(timeStr) == 5 {
			return timeStr + ":00"
		}
		return timeStr
	}
	return ""
}
