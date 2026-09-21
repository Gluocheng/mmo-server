<template>
  <n-card title="扣除道具">
    <p>按角色扣减。按道具名时自动路由背包；按槽位时需选择背包类型。</p>
    <n-space vertical>
      <n-input v-model:value="playerId" placeholder="角色 ID" />
      <n-radio-group v-model:value="mode">
        <n-radio value="item">按道具</n-radio>
        <n-radio value="slot">按槽位</n-radio>
      </n-radio-group>
      <n-select v-if="mode === 'item'" v-model:value="itemId" :options="itemOpts" placeholder="选择道具" filterable />
      <n-input v-else v-model:value="slot" placeholder="槽位（从 0 起）" />
      <n-input v-model:value="count" placeholder="数量" />
      <n-select
        v-if="mode === 'slot'"
        v-model:value="bagType"
        :options="bagOpts"
        placeholder="选择背包类型"
        filterable
      />
      <n-button type="error" :loading="busy" @click="submit">扣除</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-alert v-if="ok" type="success">{{ ok }}</n-alert>
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiPost } from '../api.js'
import { bagTypeName, bagTypeOptions, itemName, itemOptions, loadCatalog } from '../catalog.js'

const dialog = useDialog()
const playerId = ref('')
const mode = ref('item')
const itemId = ref(null)
const slot = ref('0')
const count = ref('1')
const bagType = ref(null)
const itemOpts = ref([])
const bagOpts = ref([])
const busy = ref(false)
const error = ref('')
const ok = ref('')

onMounted(() => {
  loadCatalog()
    .then(() => {
      itemOpts.value = itemOptions()
      bagOpts.value = bagTypeOptions(false)
    })
    .catch((e) => {
      error.value = e.message || '加载配表失败'
    })
})

async function submit() {
  error.value = ''
  ok.value = ''
  const pid = Number(playerId.value)
  const n = Number(count.value || '1')
  if (!pid) {
    error.value = '角色 ID 必填'
    return
  }
  if (mode.value === 'item') {
    const iid = Number(itemId.value)
    if (!iid) {
      error.value = '请选择道具'
      return
    }
    dialog.warning({
      title: '确认扣除',
      content: `从角色 ${pid} 扣除「${itemName(iid)}」x ${n}？`,
      positiveText: '确认',
      negativeText: '取消',
      onPositiveClick: () => doDeduct({ playerId: pid, itemId: iid, count: n }),
    })
    return
  }
  if (slot.value.trim() === '' || Number.isNaN(Number(slot.value))) {
    error.value = '槽位必填'
    return
  }
  const bt = Number(bagType.value)
  if (!bt) {
    error.value = '按槽扣除必须选择背包类型'
    return
  }
  const sl = Number(slot.value)
  dialog.warning({
    title: '确认扣除',
    content: `从角色 ${pid}「${bagTypeName(bt)}」槽 ${sl} 扣除 x ${n}？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doDeduct({ playerId: pid, slot: sl, count: n, bagType: bt }),
  })
}

async function doDeduct(body) {
  busy.value = true
  try {
    const data = await apiPost('/gm/bag/deduct', body)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    const pushed = data.pushed ? '已推送给在线玩家' : '玩家不在线，仅改库'
    ok.value = `扣除成功，背包「${bagTypeName(data.bagType)}」，${pushed}`
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
