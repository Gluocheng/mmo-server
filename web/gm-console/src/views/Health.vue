<template>
  <n-card title="节点健康">
    <p>调用 GET /gm/health，无需登录。</p>
    <n-space vertical>
      <n-button :loading="busy" @click="load">刷新</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-descriptions v-if="info" bordered :column="1" size="small" label-placement="left">
        <n-descriptions-item label="状态">{{ info.message || 'ok' }}</n-descriptions-item>
        <n-descriptions-item label="NATS">
          <n-tag :type="info.natsConnected ? 'success' : 'error'" size="small">
            {{ info.natsConnected ? '已连接' : '未连接' }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="远程主题">{{ info.remoteSubject || '—' }}</n-descriptions-item>
        <n-descriptions-item label="目标路径">{{ info.targetPath || '—' }}</n-descriptions-item>
      </n-descriptions>
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { apiGet } from '../api.js'

const busy = ref(false)
const error = ref('')
const info = ref(null)

async function load() {
  error.value = ''
  busy.value = true
  try {
    const data = await apiGet('/gm/health')
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      info.value = null
      return
    }
    info.value = data
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
