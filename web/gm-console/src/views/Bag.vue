<template>
  <n-card title="查背包">
    <n-space vertical>
      <n-input v-model:value="playerId" placeholder="playerId" />
      <n-input v-model:value="bagType" placeholder="bagType" />
      <n-button type="primary" :loading="busy" @click="query">查询</n-button>
      <n-data-table v-if="items.length" :columns="columns" :data="items" />
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="text" class="result-pre">{{ text }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { ref } from 'vue'
import { apiGet, formatResult } from '../api.js'

const playerId = ref('')
const bagType = ref('1')
const busy = ref(false)
const text = ref('')
const error = ref('')
const items = ref([])
const columns = [
  { title: 'slot', key: 'slot' },
  { title: 'itemId', key: 'itemId' },
  { title: 'count', key: 'count' },
  { title: 'bagType', key: 'bagType' },
]

async function query() {
  error.value = ''
  items.value = []
  if (!playerId.value.trim() || !bagType.value.trim()) {
    error.value = 'playerId 与 bagType 必填'
    return
  }
  busy.value = true
  try {
    const data = await apiGet(
      `/gm/bag?playerId=${encodeURIComponent(playerId.value.trim())}&bagType=${encodeURIComponent(bagType.value.trim())}`
    )
    text.value = formatResult(data)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
    } else if (Array.isArray(data.items)) {
      items.value = data.items
    }
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
