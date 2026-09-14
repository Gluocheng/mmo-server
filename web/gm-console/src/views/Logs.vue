<template>
  <n-card title="操作记录">
    <n-space style="margin-bottom: 12px" align="center">
      <n-input v-model:value="operator" placeholder="操作者" clearable style="width: 160px" />
      <n-input v-model:value="action" placeholder="action，如 bag.grant" clearable style="width: 180px" />
      <n-date-picker
        v-model:value="range"
        type="datetimerange"
        clearable
        start-placeholder="开始时间"
        end-placeholder="结束时间"
        style="width: 360px"
      />
      <n-button type="primary" :loading="busy" @click="load(1)">查询</n-button>
    </n-space>
    <n-data-table :columns="columns" :data="rows" :loading="busy" :pagination="false" />
    <n-pagination
      v-if="total > 0"
      v-model:page="page"
      :item-count="total"
      :page-size="pageSize"
      style="margin-top: 12px"
      @update:page="load"
    />
    <n-alert v-if="error" type="error" style="margin-top: 12px">{{ error }}</n-alert>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { apiGet, formatUnix } from '../api.js'

const operator = ref('')
const action = ref('')
const range = ref(null)
const busy = ref(false)
const error = ref('')
const rows = ref([])
const page = ref(1)
const pageSize = 20
const total = ref(0)

const columns = [
  { title: '时间', key: 'createdAtUnix', width: 180, render: (row) => formatUnix(row.createdAtUnix) },
  { title: '操作者', key: 'operator', width: 120 },
  { title: '动作', key: 'action', width: 140 },
  { title: 'uid', key: 'targetUid', width: 90 },
  { title: 'playerId', key: 'targetPlayerId', width: 100 },
  { title: '结果码', key: 'resultCode', width: 80 },
  { title: '详情', key: 'detail' },
]

async function load(p) {
  if (typeof p === 'number') {
    page.value = p
  }
  error.value = ''
  busy.value = true
  try {
    const qs = new URLSearchParams()
    if (operator.value.trim()) qs.set('operator', operator.value.trim())
    if (action.value.trim()) qs.set('action', action.value.trim())
    if (Array.isArray(range.value) && range.value.length === 2) {
      qs.set('from', String(Math.floor(range.value[0] / 1000)))
      qs.set('to', String(Math.floor(range.value[1] / 1000)))
    }
    qs.set('page', String(page.value))
    qs.set('pageSize', String(pageSize))
    const data = await apiGet(`/gm/ops/logs?${qs.toString()}`)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      rows.value = []
      total.value = 0
      return
    }
    rows.value = data.list || []
    total.value = data.total || 0
    page.value = data.page || page.value
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

onMounted(() => load(1))
</script>
