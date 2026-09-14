<template>
  <n-card title="查账号">
    <n-space vertical>
      <n-radio-group v-model:value="mode">
        <n-radio value="nickname">nickname</n-radio>
        <n-radio value="uid">uid</n-radio>
      </n-radio-group>
      <n-input
        v-if="mode === 'nickname'"
        v-model:value="nickname"
        placeholder="nickname"
        @keyup.enter="query"
      />
      <n-input v-else v-model:value="uid" placeholder="uid" @keyup.enter="query" />
      <n-button type="primary" :loading="busy" @click="query">查询</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="text" class="result-pre">{{ text }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { ref } from 'vue'
import { apiGet, formatResult } from '../api.js'

const mode = ref('nickname')
const nickname = ref('')
const uid = ref('')
const busy = ref(false)
const text = ref('')
const error = ref('')

async function query() {
  error.value = ''
  if ((mode.value === 'nickname' && !nickname.value.trim()) || (mode.value === 'uid' && !uid.value.trim())) {
    error.value = '请填写查询条件'
    return
  }
  const qs =
    mode.value === 'nickname'
      ? `nickname=${encodeURIComponent(nickname.value.trim())}`
      : `uid=${encodeURIComponent(uid.value.trim())}`
  busy.value = true
  try {
    const data = await apiGet(`/gm/account?${qs}`)
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
