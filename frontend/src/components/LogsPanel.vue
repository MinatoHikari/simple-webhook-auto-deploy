<template>
  <div class="card logs-panel">
    <div class="logs-toolbar">
      <div class="level-filter">
        <label
          v-for="lv in ['info', 'warn', 'error']"
          :key="lv"
          class="level-check"
          :class="['lc-' + lv, { on: levels[lv] }]"
        >
          <input v-model="levels[lv]" type="checkbox" />
          {{ lv }}
        </label>
      </div>
      <input v-model="query" class="search" placeholder="搜索关键词…" />
      <select v-if="project" v-model="deployment">
        <option value="">全部会话</option>
        <option v-for="d in reversedDeployments" :key="d.id" :value="d.id">
          {{ fmtDate(d.start) }} {{ shortId(d.id) }} · {{ d.result === 'success' ? '成功' : '失败' }}
        </option>
      </select>
      <select v-model="range">
        <option value="all">全部时间</option>
        <option value="30m">最近 30 分钟</option>
        <option value="1h">最近 1 小时</option>
        <option value="24h">最近 24 小时</option>
      </select>
      <div class="toolbar-right">
        <span class="sse-dot" :class="sseState" :title="sseState"></span>
        <span class="muted small">{{ sseText }}</span>
        <label class="check"><input v-model="autoScroll" type="checkbox" />自动滚动</label>
        <label class="check"><input v-model="wrap" type="checkbox" />换行</label>
        <button class="btn btn-ghost btn-sm" @click="clearScreen">清屏</button>
        <button class="btn btn-ghost btn-sm" @click="download">下载</button>
      </div>
    </div>

    <div ref="logView" class="log-view" :class="{ wrap }" @scroll="onScroll">
      <div v-if="displayList.length === 0 && !loadingHistory" class="log-empty">
        暂无日志{{ hasFilter ? '(当前筛选条件下)' : '' }}
      </div>
      <template v-for="(row, i) in displayList" :key="i">
        <div v-if="row.type === 'session'" class="log-session">
          <span>── 部署 {{ row.id }}</span>
          <span v-if="row.trigger">触发: {{ triggerText(row.trigger) }}</span>
          <span v-if="row.duration">耗时 {{ fmtDuration(row.duration) }}</span>
          <span v-if="row.result" :class="'sess-result-' + row.result">
            {{ row.result === 'success' ? '✓ 成功' : '✗ 失败' }}
          </span>
        </div>
        <div v-else class="log-line" :class="'log-' + row.level">
          <span class="log-time">{{ fmtTime(row.time) }}</span>
          <span class="log-level" :class="'lv-' + row.level">{{ row.level.toUpperCase() }}</span>
          <span v-if="!project && row.project" class="log-stage">{{ row.project }}</span>
          <span v-if="row.stage" class="log-stage">{{ row.stage }}</span>
          <span class="log-msg">{{ row.message }}</span>
        </div>
      </template>
    </div>

    <div class="logs-footer">
      <span>共 {{ entries.length }} 条</span>
      <span v-if="loadingHistory">加载历史中…</span>
      <span v-else class="muted">历史 + 实时(SSE)</span>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { api } from '../api.js'
import { connectLogStream } from '../sse.js'
import { fmtTime, fmtDate, fmtDuration, triggerText } from '../format.js'

const props = defineProps({
  project: { type: String, default: '' }, // 空 = 系统日志
  deployments: { type: Array, default: () => [] },
  initialDeployment: { type: String, default: '' }
})

const MAX_RENDER = 3000

const levels = reactive({ info: true, warn: true, error: true })
const query = ref('')
const deployment = ref(props.initialDeployment || '')
const range = ref('all')
const entries = ref([])
const sseState = ref('connecting')
const autoScroll = ref(true)
const wrap = ref(false)
const loadingHistory = ref(false)
const logView = ref(null)

let closeStream = null
let reloadTimer = null

const reversedDeployments = computed(() => [...props.deployments].reverse())

const activeLevels = computed(() =>
  ['info', 'warn', 'error'].filter(lv => levels[lv])
)

const hasFilter = computed(
  () => query.value || deployment.value || range.value !== 'all' || activeLevels.value.length < 3
)

const sseText = computed(
  () => ({ connected: '实时', connecting: '连接中', reconnecting: '重连中' }[sseState.value] || sseState.value)
)

function rangeSince() {
  const map = { '30m': 30 * 60e3, '1h': 3600e3, '24h': 24 * 3600e3 }
  const ms = map[range.value]
  return ms ? new Date(Date.now() - ms).toISOString() : ''
}

function filterParams() {
  const p = {
    q: query.value.trim(),
    deployment: deployment.value,
    limit: 1000
  }
  // 全选级别时不下发 level 参数(服务端视为不过滤)
  if (activeLevels.value.length > 0 && activeLevels.value.length < 3) {
    p.level = activeLevels.value.join(',')
  }
  const since = rangeSince()
  if (since) p.since = since
  return p
}

function streamUrl() {
  const p = filterParams()
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(p)) {
    if (k !== 'limit' && v) qs.set(k, v)
  }
  if (activeLevels.value.length === 0) qs.set('level', 'none')
  const s = qs.toString()
  return props.project
    ? `/api/projects/${encodeURIComponent(props.project)}/logs/stream${s ? '?' + s : ''}`
    : `/api/logs/stream${s ? '?' + s : ''}`
}

async function reloadHistory() {
  if (activeLevels.value.length === 0) {
    entries.value = []
    return
  }
  loadingHistory.value = true
  try {
    const res = props.project
      ? await api.projectLogs(props.project, filterParams())
      : await api.systemLogs(filterParams())
    entries.value = res.entries || []
    scrollToBottom()
  } catch { /* 忽略,由 api 层统一处理 401 */ }
  loadingHistory.value = false
}

function reconnect() {
  if (closeStream) closeStream()
  sseState.value = 'connecting'
  closeStream = connectLogStream(streamUrl(), {
    onEntry: e => {
      if (activeLevels.value.length === 0) return
      entries.value.push(e)
      if (entries.value.length > MAX_RENDER) {
        entries.value.splice(0, entries.value.length - MAX_RENDER)
      }
      if (autoScroll.value) scrollToBottom()
    },
    onState: s => { sseState.value = s }
  })
}

// 筛选条件变化:防抖后重新拉历史 + 重连 SSE(关键词输入不打爆后端)
watch(
  () => [query.value, deployment.value, range.value, activeLevels.value.join(',')],
  () => {
    clearTimeout(reloadTimer)
    reloadTimer = setTimeout(() => {
      reloadHistory()
      reconnect()
    }, 350)
  }
)

// 渲染列表:在部署会话切换处插入分隔行
const displayList = computed(() => {
  const rows = []
  let prevDep = null
  for (const e of entries.value) {
    if (props.project && e.deployment && e.deployment !== prevDep) {
      const rec = props.deployments.find(d => d.id === e.deployment)
      rows.push({
        type: 'session',
        id: e.deployment,
        trigger: rec?.trigger,
        duration: rec?.duration,
        result: rec?.result
      })
    }
    prevDep = e.deployment || prevDep
    rows.push(e)
  }
  return rows
})

function scrollToBottom() {
  nextTick(() => {
    if (logView.value) logView.value.scrollTop = logView.value.scrollHeight
  })
}

function onScroll() {
  const el = logView.value
  if (!el) return
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  autoScroll.value = atBottom
}

function clearScreen() {
  entries.value = []
}

function shortId(id) {
  return String(id || '').slice(9)
}

function download() {
  const lines = entries.value.map(
    e => `${fmtTime(e.time)} [${e.level.toUpperCase()}]${e.stage ? ' [' + e.stage + ']' : ''} ${e.message}`
  )
  const blob = new Blob([lines.join('\n')], { type: 'text/plain;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `${props.project || 'system'}-logs.txt`
  a.click()
  URL.revokeObjectURL(a.href)
}

onMounted(() => {
  reloadHistory()
  reconnect()
})
onUnmounted(() => {
  clearTimeout(reloadTimer)
  if (closeStream) closeStream()
})
</script>
