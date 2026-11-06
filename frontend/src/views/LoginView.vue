<template>
  <div class="login-wrap">
    <form class="login-card" @submit.prevent="submit">
      <h1>▲ 部署中心</h1>
      <p class="muted small" style="text-align: center; margin: 0;">webhook 自动部署服务</p>
      <input v-model="password" type="password" placeholder="管理密码" autofocus autocomplete="current-password" />
      <div v-if="error" class="error-text" style="text-align: center;">{{ error }}</div>
      <button class="btn btn-primary" type="submit" :disabled="loading || !password">
        {{ loading ? '登录中…' : '登 录' }}
      </button>
    </form>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../api.js'

const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  if (!password.value) return
  loading.value = true
  error.value = ''
  try {
    await api.login(password.value)
    window.location.href = '/'
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>
