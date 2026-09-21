<template>
  <n-card title="发放道具">
    <p>按角色入库。背包类型按道具配表自动路由，无需手填。</p>
    <n-space vertical>
      <n-input v-model:value="playerId" placeholder="角色 ID" />
      <n-select v-model:value="itemId" :options="itemOpts" placeholder="选择道具" filterable />
      <n-input v-model:value="count" placeholder="数量" />
      <n-button type="primary" :loading="busy" @click="submit">发放</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-alert v-if="ok" type="success">{{ ok }}</n-alert>
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiPost } from '../api.js'
import { bagTypeName, itemName, itemOptions, loadCatalog } from '../catalog.js'

const dialog = useDialog()
const playerId = ref('')
const itemId = ref(null)
const count = ref('1')
const itemOpts = ref([])
const busy = ref(false)
const error = ref('')
const ok = ref('')

onMounted(() => {
  loadCatalog()
    .then(() => {
      itemOpts.value = itemOptions()
    })
    .catch((e) => {
      error.value = e.message || '加载配表失败'
    })
})

async function submit() {
  error.value = ''
  ok.value = ''
  const pid = Number(playerId.value)
  const iid = Number(itemId.value)
  const n = Number(count.value || '1')
  if (!pid || !iid) {
    error.value = '角色 ID 与道具必填'
    return
  }
  dialog.warning({
    title: '确认发放',
    content: `向角色 ${pid} 发放「${itemName(iid)}」x ${n}？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doGrant(pid, iid, n),
  })
}

async function doGrant(pid, iid, n) {
  busy.value = true
  try {
    const data = await apiPost('/gm/bag/grant', { playerId: pid, itemId: iid, count: n })
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    const pushed = data.pushed ? '已推送给在线玩家' : '玩家不在线，仅入库'
    ok.value = `发放成功，进入「${bagTypeName(data.bagType)}」，${pushed}`
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
