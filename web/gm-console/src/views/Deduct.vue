<template>
  <n-card title="扣除道具">
    <p>按 playerId 扣减。itemId 与 slot 二选一；按 itemId 时可留空 bagType 自动路由。</p>
    <n-space vertical>
      <n-input v-model:value="playerId" placeholder="playerId" />
      <n-radio-group v-model:value="mode">
        <n-radio value="item">按 itemId</n-radio>
        <n-radio value="slot">按 slot</n-radio>
      </n-radio-group>
      <n-input v-if="mode === 'item'" v-model:value="itemId" placeholder="itemId" />
      <n-input v-else v-model:value="slot" placeholder="slot（从 0 起）" />
      <n-input v-model:value="count" placeholder="count" />
      <n-input v-model:value="bagType" :placeholder="mode === 'item' ? 'bagType 可选，空=自动' : 'bagType 必填'" />
      <n-button type="error" :loading="busy" @click="submit">扣除</n-button>
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
const mode = ref('item')
const itemId = ref('')
const slot = ref('0')
const count = ref('1')
const bagType = ref('')
const busy = ref(false)
const text = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  const pid = Number(playerId.value)
  const n = Number(count.value || '1')
  const bt = bagType.value.trim() === '' ? 0 : Number(bagType.value)
  if (!pid) {
    error.value = 'playerId 必填'
    return
  }
  if (mode.value === 'item') {
    const iid = Number(itemId.value)
    if (!iid) {
      error.value = 'itemId 必填'
      return
    }
    dialog.warning({
      title: '确认扣除',
      content: `从角色 ${pid} 扣除道具 ${iid} x ${n}？`,
      positiveText: '确认',
      negativeText: '取消',
      onPositiveClick: () => doDeduct({ playerId: pid, itemId: iid, count: n, bagType: bt }),
    })
    return
  }
  if (slot.value.trim() === '' || Number.isNaN(Number(slot.value))) {
    error.value = 'slot 必填'
    return
  }
  if (!bt) {
    error.value = '按槽扣除必须填写 bagType'
    return
  }
  const sl = Number(slot.value)
  dialog.warning({
    title: '确认扣除',
    content: `从角色 ${pid} 背包 ${bt} 槽 ${sl} 扣除 x ${n}？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doDeduct({ playerId: pid, slot: sl, count: n, bagType: bt }),
  })
}

async function doDeduct(body) {
  busy.value = true
  try {
    if (!body.bagType) {
      delete body.bagType
    }
    const data = await apiPost('/gm/bag/deduct', body)
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
