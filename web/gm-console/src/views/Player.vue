<template>
  <n-card title="查角色">
    <n-space vertical>
      <n-radio-group v-model:value="mode">
        <n-radio value="playerId">playerId</n-radio>
        <n-radio value="name">角色名</n-radio>
        <n-radio value="uid">uid</n-radio>
      </n-radio-group>
      <n-input v-model:value="value" :placeholder="mode" @keyup.enter="query" />
      <n-button type="primary" :loading="busy" @click="query">查询</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-data-table v-if="rows.length" :columns="columns" :data="rows" :bordered="true" />
      <n-empty v-else-if="queried && !error" description="没有角色" />
    </n-space>
  </n-card>
</template>

<script setup>
import { ref } from 'vue'
import { apiGet, formatUnix } from '../api.js'

const mode = ref('playerId')
const value = ref('')
const busy = ref(false)
const error = ref('')
const queried = ref(false)
const rows = ref([])
const columns = [
  { title: '角色 ID', key: 'playerId' },
  { title: '账号 UID', key: 'uid' },
  { title: '角色名', key: 'name' },
  { title: '已删除', key: 'deleted', render(row) { return row.deleted ? '是' : '否' } },
  { title: '创建时间', key: 'createdAtUnix', render: (row) => formatUnix(row.createdAtUnix) || '—' },
]

async function query() {
  error.value = ''
  rows.value = []
  queried.value = false
  if (!value.value.trim()) {
    error.value = '请填写查询条件'
    return
  }
  busy.value = true
  try {
    const data = await apiGet(`/gm/player?${mode.value}=${encodeURIComponent(value.value.trim())}`)
    queried.value = true
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
    } else if (Array.isArray(data.list)) {
      rows.value = data.list
    }
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
