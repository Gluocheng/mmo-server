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
      <n-input v-model:value="reason" placeholder="封号原因（可选）" />
      <n-space>
        <n-button type="error" :disabled="!accountUid" :loading="busy" @click="onBan">封号</n-button>
        <n-button :disabled="!accountUid" :loading="busy" @click="onUnban">解封</n-button>
      </n-space>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="text" class="result-pre">{{ text }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiGet, apiPost, formatResult } from '../api.js'

const dialog = useDialog()
const mode = ref('nickname')
const nickname = ref('')
const uid = ref('')
const reason = ref('')
const busy = ref(false)
const text = ref('')
const error = ref('')
const last = ref(null)

const accountUid = computed(() => (last.value && last.value.uid ? last.value.uid : 0))

async function query() {
  error.value = ''
  last.value = null
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
      return
    }
    last.value = data
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

function onBan() {
  const id = accountUid.value
  dialog.warning({
    title: '确认封号',
    content: `永久封禁 uid=${id}？在线连接会被踢下线。`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doBan(true, id),
  })
}

function onUnban() {
  const id = accountUid.value
  dialog.warning({
    title: '确认解封',
    content: `解除 uid=${id} 的封禁？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doBan(false, id),
  })
}

async function doBan(ban, id) {
  error.value = ''
  busy.value = true
  try {
    const path = ban ? '/gm/account/ban' : '/gm/account/unban'
    const body = { uid: id }
    if (ban && reason.value.trim()) {
      body.reason = reason.value.trim()
    }
    const data = await apiPost(path, body)
    text.value = formatResult(data)
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
    } else {
      await query()
    }
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}
</script>
