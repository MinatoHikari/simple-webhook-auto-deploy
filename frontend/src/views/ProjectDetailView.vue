<template>
  <div v-if="detail">
    <div class="page-head">
      <div>
        <router-link to="/" class="muted small">← 返回项目列表</router-link>
        <h2 style="display: flex; align-items: center; gap: 10px;">
          {{ name }}
          <span class="badge" :class="'badge-' + detail.status.status">{{ statusText(detail.status.status) }}</span>
        </h2>
      </div>
      <button
        class="btn btn-primary"
        :disabled="detail.status.status === 'deploying'"
        @click="deploy"
      >
        {{ detail.status.status === 'deploying' ? '部署中…' : '立即部署' }}
      </button>
    </div>

    <div class="tabs">
      <button :class="{ active: tab === 'overview' }" @click="tab = 'overview'">概览</button>
      <button :class="{ active: tab === 'config' }" @click="tab = 'config'">配置</button>
      <button :class="{ active: tab === 'logs' }" @click="tab = 'logs'">日志</button>
    </div>

    <!-- 概览 -->
    <div v-if="tab === 'overview'" class="card">
      <div class="overview-grid">
        <div class="overview-item"><span class="k">仓库路径</span><span class="v">{{ detail.cfg.path }}</span></div>
        <div class="overview-item"><span class="k">分支过滤</span><span class="v">{{ detail.cfg.branch || '不过滤' }}</span></div>
        <div class="overview-item">
          <span class="k">构建命令</span>
          <span class="v">{{ detail.cfg.packageManager || 'npm' }} run {{ detail.cfg.script || 'build' }}</span>
        </div>
        <div class="overview-item"><span class="k">产物目录</span><span class="v">{{ detail.cfg.dist || './dist' }}</span></div>
        <div class="overview-item"><span class="k">发布到</span><span class="v">{{ detail.cfg.output || '不发布' }}</span></div>
        <div class="overview-item">
          <span class="k">webhook</span>
          <span class="v">POST /webhook/{{ detail.cfg.name }}{{ detail.cfg.token ? '(独立 token)' : '(全局 token)' }}</span>
        </div>
        <div v-if="detail.cfg.additionalScript" class="overview-item">
          <span class="k">附加脚本</span>
          <span class="v">{{ detail.cfg.additionalScript }} {{ (detail.cfg.additionalScriptArgs || []).join(' ') }}</span>
        </div>
      </div>

      <h3 class="section-title">切换分支</h3>
      <form class="branch-form" @submit.prevent="switchBranch">
        <input v-model="newBranch" placeholder="目标分支名" />
        <button class="btn" type="submit" :disabled="!newBranch || detail.status.status === 'deploying'">切换</button>
      </form>

      <h3 class="section-title">部署历史</h3>
      <table v-if="detail.status.history.length">
        <thead>
          <tr><th>部署会话</th><th>开始时间</th><th>触发</th><th>分支</th><th>耗时</th><th>结果</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="rec in [...detail.status.history].reverse()" :key="rec.id">
            <td class="mono small">{{ rec.id }}</td>
            <td class="small">{{ fmtDateTime(rec.start) }}</td>
            <td>{{ triggerText(rec.trigger) }}</td>
            <td class="mono small">{{ rec.branch || '-' }}</td>
            <td>{{ fmtDuration(rec.duration) }}</td>
            <td>
              <span class="badge" :class="rec.result === 'success' ? 'badge-success' : 'badge-failed'">
                {{ rec.result === 'success' ? '成功' : '失败' }}
              </span>
            </td>
            <td><a href="javascript:void(0)" @click="viewSession(rec)">查看日志</a></td>
          </tr>
        </tbody>
      </table>
      <div v-else class="muted small">暂无部署记录</div>
    </div>

    <!-- 配置 -->
    <div v-if="tab === 'config'" class="card">
      <ProjectForm :initial="detail.cfg" submit-label="保存配置" @submit="save" />
      <div class="danger-zone">
        <span class="muted">删除后将从配置文件移除该项目(磁盘日志文件保留)</span>
        <button class="btn btn-danger" @click="remove">删除项目</button>
      </div>
    </div>

    <!-- 日志 -->
    <LogsPanel
      v-if="tab === 'logs'"
      :key="logsKey"
      :project="name"
      :deployments="detail.status.history"
      :initial-deployment="selectedDeployment"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { statusText, fmtDateTime, fmtDuration, triggerText } from '../format.js'
import { toast } from '../toast.js'
import ProjectForm from '../components/ProjectForm.vue'
import LogsPanel from '../components/LogsPanel.vue'

const props = defineProps({ name: String })

const detail = ref(null)
const tab = ref('overview')
const newBranch = ref('')
const selectedDeployment = ref('')
const logsKey = ref(0)
let timer = null

async function refresh() {
  try {
    detail.value = await api.getProject(props.name)
  } catch { /* 401 由 api 层处理 */ }
}

async function deploy() {
  try {
    const res = await api.deploy(props.name)
    toast(res.result === 'started' ? '已开始部署' : '已加入部署队列', 'success')
    refresh()
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function switchBranch() {
  try {
    await api.switchBranch(props.name, newBranch.value)
    toast(`已切换到 ${newBranch.value}`, 'success')
    newBranch.value = ''
    refresh()
  } catch (e) {
    toast(e.message, 'error')
  }
}

function viewSession(rec) {
  selectedDeployment.value = rec.id
  logsKey.value++ // 强制重建日志面板并应用会话筛选
  tab.value = 'logs'
}

async function save(cfg) {
  try {
    await api.updateProject(props.name, cfg)
    toast('配置已保存', 'success')
    refresh()
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function remove() {
  if (!window.confirm(`确定删除项目 ${props.name} 吗?`)) return
  try {
    await api.deleteProject(props.name)
    toast('项目已删除', 'success')
    window.location.href = '/'
  } catch (e) {
    toast(e.message, 'error')
  }
}

onMounted(() => {
  refresh()
  timer = setInterval(refresh, 2000)
})
onUnmounted(() => clearInterval(timer))
</script>
