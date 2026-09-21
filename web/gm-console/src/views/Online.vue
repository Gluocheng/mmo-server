<template>
  <n-card title="场景在线">
    <p>仅已进场玩家。sceneId 留空为全部场景。</p>
    <n-space vertical>
      <n-input v-model:value="sceneId" placeholder="sceneId 可选" />
      <n-button type="primary" :loading="busy" @click="load">刷新</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-data-table :columns="columns" :data="rows" :loading="busy" />
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { apiGet } from '../api.js'

const sceneId = ref('')
const busy = ref(false)
const error = ref('')
const rows = ref([])
const columns = [
  { title: 'uid', key: 'uid' },
  { title: 'sceneId', key: 'sceneId' },
]

async function load() {
  error.value = ''
  const qs = sceneId.value.trim() ? `?sceneId=${encodeURIComponent(sceneId.value.trim())}` : ''
  busy.value = true
  try {
    const data = await apiGet(`/gm/world/online${qs}`)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      rows.value = []
      return
    }
    rows.value = data.list || []
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
