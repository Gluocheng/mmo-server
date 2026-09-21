<template>
  <n-card title="配表热更">
    <p>空表名表示全量重载。需 profile 开启 gameconfig.allow_reload。</p>
    <n-space vertical>
      <n-input v-model:value="tableName" placeholder="空 = 全部，或 item / bag_type" />
      <n-button type="error" :loading="busy" @click="submit">重载</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-alert v-if="ok" type="success">{{ ok }}</n-alert>
    </n-space>
  </n-card>
</template>

<script setup>
import { ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiPost } from '../api.js'

const dialog = useDialog()
const tableName = ref('')
const busy = ref(false)
const error = ref('')
const ok = ref('')

async function submit() {
  error.value = ''
  ok.value = ''
  const name = tableName.value.trim()
  dialog.warning({
    title: '确认热更',
    content: name ? `确认重载配表 ${name}？` : '确认全量重载全部配表？',
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doReload(name),
  })
}

async function doReload(name) {
  busy.value = true
  try {
    const data = await apiPost('/gm/config/reload', { tableName: name })
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    ok.value = `重载成功，版本 ${data.version || '—'}，表数量 ${data.tables ?? '—'}`
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
