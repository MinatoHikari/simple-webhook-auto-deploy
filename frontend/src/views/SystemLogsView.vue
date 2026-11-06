<template>
  <div>
    <div class="page-head">
      <h2>系统日志</h2>
      <span class="muted small">服务自身事件 + 所有项目的日志流</span>
    </div>
    <LogsPanel :project="''" :deployments="[]" />
  </div>
</template>

<script setup>
import LogsPanel from '../components/LogsPanel.vue'
</script>
