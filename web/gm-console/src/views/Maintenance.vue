<template>
  <n-card title="强制维护">
    <p>开启后拒绝签发/鉴权/刷新 token，并踢光所有网关连接。关闭后旧 token 仍可用。</p>
    <n-space vertical>
      <n-button :loading="busy" @click="load">刷新状态</n-button>
      <n-descriptions v-if="info" bordered :column="1" size="small">
        <n-descriptions-item label="维护中">{{ info.enabled ? '是' : '否' }}</n-descriptions-item>
        <n-descriptions-item label="原因">{{ info.reason || '—' }}</n-descriptions-item>
      </n-descriptions>
      <n-input v-model:value="reason" placeholder="维护原因（可选）" />
      <n-space>
        <n-button type="error" :loading="busy" @click="onEnable">开启维护</n-button>
        <n-button :loading="busy" @click="onDisable">关闭维护</n-button>
      </n-space>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="text" class="result-pre">{{ text }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiGet, apiPost, formatResult } from '../api.js'

const dialog = useDialog()
const busy = ref(false)
const error = ref('')
const text = ref('')
const info = ref(null)
const reason = ref('')

async function load() {
  error.value = ''
  busy.value = true
  try {
    const data = await apiGet('/gm/maintenance')
    text.value = formatResult(data)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    info.value = data
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

function onEnable() {
  dialog.warning({
    title: '确认开启维护',
    content: '将拒绝所有登录并踢下全部在线连接。',
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => setEnabled(true),
  })
}

function onDisable() {
  dialog.warning({
    title: '确认关闭维护',
    content: '关闭后玩家可用未过期 token 重新进入。',
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => setEnabled(false),
  })
}

async function setEnabled(enabled) {
  error.value = ''
  busy.value = true
  try {
    const body = { enabled }
    if (enabled && reason.value.trim()) {
      body.reason = reason.value.trim()
    }
    const data = await apiPost('/gm/maintenance', body)
    text.value = formatResult(data)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    info.value = data
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
