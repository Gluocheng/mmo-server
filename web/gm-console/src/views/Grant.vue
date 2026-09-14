<template>
  <n-card title="发放道具">
    <p>按 playerId 入库。bagType 留空则按道具配置自动路由。</p>
    <n-space vertical>
      <n-input v-model:value="playerId" placeholder="playerId" />
      <n-input v-model:value="itemId" placeholder="itemId" />
      <n-input v-model:value="count" placeholder="count" />
      <n-input v-model:value="bagType" placeholder="bagType 可选，空=自动" />
      <n-button type="primary" :loading="busy" @click="submit">发放</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="text" class="result-pre">{{ text }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiPost, formatResult } from '../api.js'

const dialog = useDialog()
const playerId = ref('')
const itemId = ref('')
const count = ref('1')
const bagType = ref('')
const busy = ref(false)
const text = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  const pid = Number(playerId.value)
  const iid = Number(itemId.value)
  const n = Number(count.value || '1')
  const bt = bagType.value.trim() === '' ? 0 : Number(bagType.value)
  if (!pid || !iid) {
    error.value = 'playerId 与 itemId 必填'
    return
  }
  dialog.warning({
    title: '确认发放',
    content: `向角色 ${pid} 发放道具 ${iid} x ${n}${bt ? ` 到背包 ${bt}` : ''}？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doGrant(pid, iid, n, bt),
  })
}

async function doGrant(pid, iid, n, bt) {
  busy.value = true
  try {
    const body = { playerId: pid, itemId: iid, count: n }
    if (bt > 0) {
      body.bagType = bt
    }
    const data = await apiPost('/gm/bag/grant', body)
    text.value = formatResult(data)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
    }
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
