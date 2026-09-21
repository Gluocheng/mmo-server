<template>
  <n-card title="游戏时间">
    <p>偏置为绝对值秒数（≥0）。写入 Redis 后热更新 game 与 login。</p>
    <n-space vertical>
      <n-button :loading="busy" @click="load">刷新当前时间</n-button>
      <n-descriptions v-if="info" bordered :column="1" size="small">
        <n-descriptions-item label="偏置秒">{{ info.biasSeconds }}</n-descriptions-item>
        <n-descriptions-item label="游戏时刻">{{ formatUnix(info.unixNow) }}</n-descriptions-item>
        <n-descriptions-item label="真实时刻">{{ formatUnix(info.realUnixNow) }}</n-descriptions-item>
      </n-descriptions>
      <n-input v-model:value="addSeconds" placeholder="快进秒数，如 3600" />
      <n-space>
        <n-button type="primary" :loading="busy" @click="fastForward">快进</n-button>
        <n-button :loading="busy" @click="resetBias">偏置归零</n-button>
      </n-space>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="text" class="result-pre">{{ text }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiGet, apiPost, formatResult, formatUnix } from '../api.js'

const dialog = useDialog()
const busy = ref(false)
const error = ref('')
const text = ref('')
const info = ref(null)
const addSeconds = ref('3600')

async function load() {
  error.value = ''
  busy.value = true
  try {
    const data = await apiGet('/gm/time')
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

function fastForward() {
  const n = Number(addSeconds.value)
  if (!n || n < 0) {
    error.value = '请填写大于 0 的快进秒数'
    return
  }
  const current = info.value && info.value.biasSeconds ? Number(info.value.biasSeconds) : 0
  const next = current + n
  dialog.warning({
    title: '确认快进',
    content: `将偏置设为 ${next} 秒（当前 ${current} + ${n}）？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => setBias(next),
  })
}

function resetBias() {
  dialog.warning({
    title: '确认归零',
    content: '将游戏时间偏置设为 0？',
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => setBias(0),
  })
}

async function setBias(sec) {
  error.value = ''
  busy.value = true
  try {
    const data = await apiPost('/gm/time', { biasSeconds: sec })
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
