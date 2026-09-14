<template>
  <n-card title="踢下线">
    <p>只断开当前连接，不封号、不吊销 token。uid 与 playerId 二选一。</p>
    <n-space vertical>
      <n-radio-group v-model:value="mode">
        <n-radio value="uid">uid</n-radio>
        <n-radio value="playerId">playerId</n-radio>
      </n-radio-group>
      <n-input v-model:value="value" :placeholder="mode" />
      <n-button type="error" :loading="busy" @click="submit">踢下线</n-button>
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
const mode = ref('uid')
const value = ref('')
const busy = ref(false)
const text = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  const n = Number(value.value)
  if (!n) {
    error.value = '请填写有效 ID'
    return
  }
  const msg = mode.value === 'uid' ? `确认踢下线 uid=${n}？` : `确认踢下线 playerId=${n}？`
  dialog.warning({
    title: '确认踢下线',
    content: msg,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doKick(n),
  })
}

async function doKick(n) {
  busy.value = true
  try {
    const body = mode.value === 'uid' ? { uid: n } : { playerId: n }
    const data = await apiPost('/gm/player/kick', body)
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
