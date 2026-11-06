// 时间与状态格式化工具

export function fmtTime(iso) {
  return new Date(iso).toLocaleTimeString('zh-CN', { hour12: false })
}

export function fmtDate(iso) {
  return new Date(iso).toLocaleDateString('zh-CN')
}

export function fmtDateTime(iso) {
  if (!iso || iso.startsWith('0001-01-01')) return ''
  return new Date(iso).toLocaleString('zh-CN', { hour12: false })
}

export function fmtDuration(ms) {
  if (ms == null) return ''
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(ms / 60000)
  const s = Math.round((ms % 60000) / 1000)
  return `${m}m${s}s`
}

export function fmtAgo(iso) {
  if (!iso || iso.startsWith('0001-01-01')) return ''
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

const statusMap = {
  idle: '空闲',
  queued: '排队中',
  deploying: '部署中',
  failed: '失败'
}

export function statusText(s) {
  return statusMap[s] || s
}

const triggerMap = {
  webhook: 'webhook',
  manual: '手动'
}

export function triggerText(t) {
  return triggerMap[t] || t
}
