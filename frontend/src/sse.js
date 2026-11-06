// SSE 日志流封装:断线 3 秒自动重连,透传结构化条目与连接状态
export function connectLogStream(url, { onEntry, onState } = {}) {
  let es = null
  let timer = null
  let closed = false

  function open() {
    if (closed) return
    es = new EventSource(url)
    es.onopen = () => onState && onState('connected')
    es.onerror = () => {
      onState && onState('reconnecting')
      es.close()
      timer = setTimeout(open, 3000)
    }
    es.onmessage = ev => {
      let data = null
      try {
        data = JSON.parse(ev.data)
      } catch {
        return
      }
      if (data && data.type === 'connected') {
        onState && onState('connected')
        return
      }
      if (data && data.time) {
        onEntry && onEntry(data)
      }
    }
  }

  open()

  return function close() {
    closed = true
    clearTimeout(timer)
    if (es) es.close()
  }
}
