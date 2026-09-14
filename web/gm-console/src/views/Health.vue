<template>
  <n-card title="节点健康">
    <p>调用 GET /gm/health，无需登录。</p>
    <n-button :loading="busy" @click="load">刷新</n-button>
    <n-alert v-if="error" type="error" style="margin-top: 12px">{{ error }}</n-alert>
    <pre v-if="text" class="result-pre" style="margin-top: 12px">{{ text }}</pre>
  </n-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { apiGet, formatResult } from '../api.js'

const busy = ref(false)
const text = ref('')
const error = ref('')

async function load() {
  error.value = ''
  busy.value = true
  try {
    const data = await apiGet('/gm/health')
    text.value = formatResult(data)
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
