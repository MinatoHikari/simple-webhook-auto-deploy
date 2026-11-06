<template>
  <form class="form" @submit.prevent="submit">
    <div class="field-row">
      <label class="field">
        <span>项目名 *{{ initial ? '(不可修改)' : '' }}</span>
        <input v-model="form.name" :disabled="!!initial" placeholder="如 web,用于 URL 路由与日志" />
      </label>
      <label class="field">
        <span>分支过滤</span>
        <input v-model="form.branch" placeholder="留空 = 不过滤任何分支" />
      </label>
    </div>
    <label class="field">
      <span>仓库路径 * (服务器上仓库的绝对路径)</span>
      <input v-model="form.path" placeholder="如 /srv/apps/web" />
    </label>
    <div class="field-row">
      <label class="field">
        <span>包管理器</span>
        <input v-model="form.packageManager" placeholder="npm" />
      </label>
      <label class="field">
        <span>构建脚本</span>
        <input v-model="form.script" placeholder="build" />
      </label>
    </div>
    <div class="field-row">
      <label class="field">
        <span>构建产物目录</span>
        <input v-model="form.dist" placeholder="./dist" />
      </label>
      <label class="field">
        <span>发布目标目录</span>
        <input v-model="form.output" placeholder="留空 = 只构建不发布" />
      </label>
    </div>
    <div class="field-row">
      <label class="field">
        <span>附加脚本 (构建前执行)</span>
        <input v-model="form.additionalScript" placeholder="如 /usr/bin/pm2,留空跳过" />
      </label>
      <label class="field">
        <span>附加脚本参数 (逗号分隔)</span>
        <input v-model="form.argsText" placeholder="如 restart, process.json" />
      </label>
    </div>
    <label class="field">
      <span>webhook token (留空使用全局 token)</span>
      <input v-model="form.token" placeholder="项目独立 token" />
    </label>
    <div v-if="error" class="error-text">{{ error }}</div>
    <div class="form-actions">
      <button type="button" class="btn btn-ghost" @click="$emit('cancel')">取消</button>
      <button type="submit" class="btn btn-primary">{{ submitLabel || '保存' }}</button>
    </div>
  </form>
</template>

<script setup>
import { reactive, ref } from 'vue'

const props = defineProps({
  initial: Object,
  submitLabel: String
})
const emit = defineEmits(['submit', 'cancel'])

const form = reactive({
  name: props.initial?.name ?? '',
  path: props.initial?.path ?? '',
  branch: props.initial?.branch ?? '',
  token: props.initial?.token ?? '',
  packageManager: props.initial?.packageManager ?? '',
  script: props.initial?.script ?? '',
  dist: props.initial?.dist ?? '',
  output: props.initial?.output ?? '',
  additionalScript: props.initial?.additionalScript ?? '',
  argsText: (props.initial?.additionalScriptArgs ?? []).join(',')
})
const error = ref('')

function submit() {
  const name = form.name.trim()
  if (!name) { error.value = '项目名不能为空'; return }
  if (!/^[A-Za-z0-9_-]+$/.test(name)) { error.value = '项目名只能包含字母、数字、下划线和连字符'; return }
  if (!form.path.trim()) { error.value = '仓库路径不能为空'; return }
  error.value = ''
  emit('submit', {
    name,
    path: form.path.trim(),
    branch: form.branch.trim(),
    token: form.token,
    packageManager: form.packageManager.trim(),
    script: form.script.trim(),
    dist: form.dist.trim(),
    output: form.output.trim(),
    additionalScript: form.additionalScript.trim(),
    additionalScriptArgs: form.argsText.split(',').map(s => s.trim()).filter(Boolean)
  })
}
</script>
