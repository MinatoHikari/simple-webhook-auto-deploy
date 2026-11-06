<template>
  <div>
    <div class="page-head">
      <h2>项目列表</h2>
      <button class="btn btn-primary" @click="showCreate = true">+ 新增项目</button>
    </div>

    <div v-if="!loading && projects.length === 0" class="empty">
      还没有项目。点击右上角「新增项目」,添加第一个要管理的仓库目录。
    </div>

    <div class="project-grid">
      <div
        v-for="p in projects"
        :key="p.name"
        class="card project-card"
        @click="$router.push('/projects/' + encodeURIComponent(p.name))"
      >
        <div class="project-card-head">
          <span class="project-name">{{ p.name }}</span>
          <span class="badge" :class="'badge-' + p.status">{{ statusText(p.status) }}</span>
        </div>
        <div class="mono muted small" style="word-break: break-all;">{{ p.path }}</div>
        <div class="project-meta">
          <span v-if="p.branch">分支 {{ p.branch }}</span>
          <span>{{ p.lastDeploy ? '最近部署 ' + fmtAgo(p.lastDeploy) : '尚未部署' }}</span>
          <span v-if="p.lastResult === 'failed'" style="color: var(--red);">上次失败</span>
        </div>
        <div class="project-actions">
          <button class="btn btn-primary btn-sm" @click.stop="deploy(p)">立即部署</button>
          <button class="btn btn-ghost btn-sm" @click.stop="$router.push('/projects/' + encodeURIComponent(p.name))">
            详情 / 日志
          </button>
        </div>
      </div>
    </div>

    <div v-if="showCreate" class="modal-mask" @click.self="showCreate = false">
      <div class="modal">
        <h3>新增项目</h3>
        <ProjectForm :initial="null" submit-label="创建项目" @submit="create" @cancel="showCreate = false" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { statusText, fmtAgo } from '../format.js'
import { toast } from '../toast.js'
import ProjectForm from '../components/ProjectForm.vue'

const projects = ref([])
const loading = ref(true)
const showCreate = ref(false)
let timer = null

async function refresh() {
  try {
    const list = await api.listProjects()
    // 部署中的项目状态变化最值得看,轮询兜底实时性
    projects.value = list.map(p => ({ ...p, lastDeploy: p.lastDeploy && !p.lastDeploy.startsWith('0001') ? p.lastDeploy : '' }))
  } catch { /* 401 已由 api 层处理 */ }
  loading.value = false
}

async function deploy(p) {
  try {
    const res = await api.deploy(p.name)
    toast(res.result === 'started' ? `已开始部署 ${p.name}` : `${p.name} 已加入部署队列`, 'success')
    refresh()
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function create(cfg) {
  try {
    await api.createProject(cfg)
    showCreate.value = false
    toast(`项目 ${cfg.name} 已创建`, 'success')
    refresh()
  } catch (e) {
    toast(e.message, 'error')
  }
}

onMounted(() => {
  refresh()
  timer = setInterval(refresh, 3000)
})
onUnmounted(() => clearInterval(timer))
</script>
