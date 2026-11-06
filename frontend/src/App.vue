<template>
  <div class="app">
    <header class="topbar">
      <div class="topbar-left">
        <span class="logo">▲</span>
        <router-link to="/" class="brand">部署中心</router-link>
        <nav class="nav">
          <router-link to="/">项目</router-link>
          <router-link to="/logs">系统日志</router-link>
        </nav>
      </div>
      <button v-if="authed" class="btn btn-ghost btn-sm" @click="logout">退出登录</button>
    </header>
    <main class="main">
      <router-view />
    </main>
    <div class="toasts">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="'toast-' + t.type">{{ t.message }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from './api.js'
import { toasts } from './toast.js'

const authed = ref(true)

onMounted(async () => {
  try {
    const res = await api.authCheck()
    authed.value = res.authed
  } catch {
    authed.value = false
  }
})

function logout() {
  api.logout().finally(() => {
    window.location.href = '/login'
  })
}
</script>
