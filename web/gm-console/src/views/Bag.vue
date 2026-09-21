<template>
  <n-card title="查背包">
    <n-space vertical>
      <n-input v-model:value="playerId" placeholder="角色 ID" @keyup.enter="query" />
      <n-select
        v-model:value="bagType"
        :options="bagOptions"
        placeholder="选择背包类型"
        filterable
      />
      <n-button type="primary" :loading="busy" @click="query">查询</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-data-table v-if="rows.length" :columns="columns" :data="rows" :bordered="true" />
      <n-empty v-else-if="queried && !error" description="背包为空" />
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { apiGet } from '../api.js'
import { bagTypeName, bagTypeOptions, defaultBagType, itemName, loadCatalog } from '../catalog.js'

const playerId = ref('')
const bagType = ref(null)
const bagOptions = ref([])
const busy = ref(false)
const error = ref('')
const queried = ref(false)
const rows = ref([])
const columns = [
  { title: '槽位', key: 'slot', width: 80 },
  { title: '道具', key: 'itemName' },
  { title: '道具 ID', key: 'itemId', width: 100 },
  { title: '数量', key: 'count', width: 80 },
  { title: '背包', key: 'bagName' },
]

async function ensureCatalog() {
  await loadCatalog()
  bagOptions.value = bagTypeOptions(true)
  if (bagType.value === null) {
    bagType.value = defaultBagType()
  }
}

onMounted(() => {
  ensureCatalog().catch((e) => {
    error.value = e.message || '加载配表失败'
  })
})

function decorate(item, queriedType) {
  const bt = Number(item.bagType) || Number(queriedType) || 0
  return {
    ...item,
    itemName: itemName(item.itemId),
    bagName: bagTypeName(bt),
  }
}

async function queryOne(pid, type) {
  const data = await apiGet(`/gm/bag?playerId=${encodeURIComponent(pid)}&bagType=${encodeURIComponent(type)}`)
  if (data.code && data.code !== 0) {
    throw new Error(data.message || `业务码 ${data.code}`)
  }
  const list = Array.isArray(data.items) ? data.items : []
  return list.map((it) => decorate(it, type))
}

async function query() {
  error.value = ''
  rows.value = []
  queried.value = false
  const pid = playerId.value.trim()
  if (!pid) {
    error.value = '请填写角色 ID'
    return
  }
  if (bagType.value === null || bagType.value === undefined) {
    error.value = '请选择背包类型'
    return
  }
  busy.value = true
  try {
    await ensureCatalog()
    const types =
      Number(bagType.value) === 0
        ? bagOptions.value.filter((o) => o.value > 0).map((o) => o.value)
        : [Number(bagType.value)]
    const all = []
    for (const t of types) {
      all.push(...(await queryOne(pid, t)))
    }
    rows.value = all
    queried.value = true
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
