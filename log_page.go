package main

import (
	"github.com/kataras/iris/v12"
)

// LogPage 返回日志查看页面
func LogPage(ctx iris.Context) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>实时日志查看</title>
    <style>
        body {
            font-family: 'Courier New', monospace;
            background-color: #1e1e1e;
            color: #d4d4d4;
            margin: 0;
            padding: 10px;
        }
        .container {
            max-width: 100%;
            margin: 0 auto;
        }
        h1 {
            color: #4ec9b0;
            margin: 0 0 8px 0;
            font-size: 18px;
        }
        .log-container {
            background-color: #252526;
            border: 1px solid #3e3e42;
            border-radius: 4px;
            padding: 8px;
            height: 75vh;
            overflow-y: auto;
            font-size: 12px;
            line-height: 1.4;
        }
        .log-line {
            margin: 1px 0;
            white-space: pre-wrap;
            word-wrap: break-word;
            display: flex;
            align-items: flex-start;
        }
        .log-meta {
            flex-shrink: 0;
            margin-right: 8px;
            min-width: 140px;
        }
        .log-time {
            color: #858585;
            font-size: 11px;
        }
        .log-level {
            font-weight: bold;
            font-size: 11px;
            margin-left: 4px;
        }
        .log-level.info {
            color: #4ec9b0;
        }
        .log-level.error {
            color: #f48771;
        }
        .log-level.warn {
            color: #dcdcaa;
        }
        .log-content {
            flex: 1;
            word-break: break-word;
        }
        .log-line.error .log-content {
            color: #f48771;
        }
        .log-line.info .log-content {
            color: #d4d4d4;
        }
        .log-line.warn .log-content {
            color: #dcdcaa;
        }
        .status {
            margin-bottom: 6px;
            padding: 6px 8px;
            background-color: #2d2d30;
            border-radius: 4px;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background-color: #4ec9b0;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .status-text {
            color: #4ec9b0;
            font-size: 12px;
        }
        .clear-btn {
            background-color: #0e639c;
            color: white;
            border: none;
            padding: 4px 12px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 12px;
        }
        .clear-btn:hover {
            background-color: #1177bb;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>实时日志查看</h1>
        <div class="status">
            <div class="status-dot"></div>
            <span class="status-text" id="status">连接中...</span>
            <button class="clear-btn" onclick="clearLogs()">清空日志</button>
        </div>
        <div class="log-container" id="logContainer"></div>
    </div>
    <script>
        const logContainer = document.getElementById('logContainer');
        const statusText = document.getElementById('status');
        let eventSource;

        function addLogLine(data) {
            const line = document.createElement('div');
            const level = data.level || 'info';
            const timestamp = data.timestamp || '';
            let message = data.message || '';
            
            // 确保消息中没有换行符，替换为空格
            message = message.replace(/\n/g, ' ').replace(/\r/g, ' ').replace(/\r\n/g, ' ');
            // 清理多余的空格
            message = message.replace(/\s+/g, ' ').trim();
            
            // 如果消息为空，跳过
            if (!message && !timestamp) {
                return;
            }
            
            line.className = 'log-line ' + level;
            
            // 创建元信息容器（时间和等级）
            const meta = document.createElement('span');
            meta.className = 'log-meta';
            
            const timeSpan = document.createElement('span');
            timeSpan.className = 'log-time';
            timeSpan.textContent = timestamp;
            
            const levelSpan = document.createElement('span');
            levelSpan.className = 'log-level ' + level;
            levelSpan.textContent = '[' + level.toUpperCase() + ']';
            
            meta.appendChild(timeSpan);
            meta.appendChild(levelSpan);
            
            // 创建内容容器
            const content = document.createElement('span');
            content.className = 'log-content';
            content.textContent = message;
            
            line.appendChild(meta);
            line.appendChild(content);
            logContainer.appendChild(line);
            
            // 自动滚动到底部
            logContainer.scrollTop = logContainer.scrollHeight;
        }

        function clearLogs() {
            logContainer.innerHTML = '';
        }

        function connect() {
            if (eventSource) {
                eventSource.close();
            }
            
            eventSource = new EventSource('/log/stream');
            
            eventSource.onopen = function() {
                statusText.textContent = '已连接';
                statusText.style.color = '#4ec9b0';
            };
            
            eventSource.onmessage = function(event) {
                const data = JSON.parse(event.data);
                addLogLine(data);
            };
            
            eventSource.onerror = function(event) {
                statusText.textContent = '连接断开，正在重连...';
                statusText.style.color = '#f48771';
                eventSource.close();
                setTimeout(connect, 3000);
            };
        }

        // 初始连接
        connect();
    </script>
</body>
</html>`
	ctx.HTML(html)
}

