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
      <n-input v-model:value="reason" placeholder="封号 / 禁言原因（可选）" />
      <n-input v-model:value="duration" placeholder="时长秒，空或 0=永久" />
      <n-space>
        <n-button type="error" :disabled="!accountUid" :loading="busy" @click="onBan">封号</n-button>
        <n-button :disabled="!accountUid" :loading="busy" @click="onUnban">解封</n-button>
        <n-button type="warning" :disabled="!accountUid" :loading="busy" @click="onMute">禁言</n-button>
        <n-button :disabled="!accountUid" :loading="busy" @click="onUnmute">解禁</n-button>
      </n-space>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-descriptions v-if="last" bordered :column="2" size="small" label-placement="left">
        <n-descriptions-item label="UID">{{ last.uid }}</n-descriptions-item>
        <n-descriptions-item label="昵称">{{ last.nickname || '—' }}</n-descriptions-item>
        <n-descriptions-item label="创建时间">{{ formatUnix(last.createdAtUnix) || '—' }}</n-descriptions-item>
        <n-descriptions-item label="封号状态">
          <n-tag :type="last.banned ? 'error' : 'success'" size="small">
            {{ last.banned ? '已封禁' : '正常' }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="封号截止">{{ formatLimit(last.banned, last.bannedUntil) }}</n-descriptions-item>
        <n-descriptions-item label="封号原因">{{ last.banReason || '—' }}</n-descriptions-item>
        <n-descriptions-item label="禁言状态">
          <n-tag :type="last.muted ? 'warning' : 'success'" size="small">
            {{ last.muted ? '已禁言' : '正常' }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="禁言截止">{{ formatLimit(last.muted, last.mutedUntil) }}</n-descriptions-item>
        <n-descriptions-item label="禁言原因">{{ last.muteReason || '—' }}</n-descriptions-item>
      </n-descriptions>
    </n-space>
  </n-card>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiGet, apiPost, formatUnix } from '../api.js'

const dialog = useDialog()
const mode = ref('nickname')
const nickname = ref('')
const uid = ref('')
const reason = ref('')
const duration = ref('')
const busy = ref(false)
const error = ref('')
const last = ref(null)

const accountUid = computed(() => (last.value && last.value.uid ? last.value.uid : 0))

function formatLimit(active, until) {
  if (!active) {
    return '—'
  }
  const n = Number(until) || 0
  if (n <= 0) {
    return '永久'
  }
  return formatUnix(n) || '—'
}

function parseDuration() {
  const raw = duration.value.trim()
  if (!raw) {
    return 0
  }
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 0) {
    return null
  }
  return Math.floor(n)
}

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
  const sec = parseDuration()
  if (sec === null) {
    error.value = '时长须为 ≥0 的秒数'
    return
  }
  const label = sec > 0 ? `${sec} 秒` : '永久'
  dialog.warning({
    title: '确认封号',
    content: `${label}封禁 uid=${id}？在线连接会被踢下线，token 会吊销。`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doBan(true, id, sec),
  })
}

function onUnban() {
  const id = accountUid.value
  dialog.warning({
    title: '确认解封',
    content: `解除 uid=${id} 的封禁？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doBan(false, id, 0),
  })
}

function onMute() {
  const id = accountUid.value
  const sec = parseDuration()
  if (sec === null) {
    error.value = '时长须为 ≥0 的秒数'
    return
  }
  const label = sec > 0 ? `${sec} 秒` : '永久'
  dialog.warning({
    title: '确认禁言',
    content: `${label}禁言 uid=${id}？玩家仍可在场景内，但不能发聊天。`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doMute(true, id, sec),
  })
}

function onUnmute() {
  const id = accountUid.value
  dialog.warning({
    title: '确认解禁',
    content: `解除 uid=${id} 的禁言？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doMute(false, id, 0),
  })
}

async function doBan(ban, id, sec) {
  error.value = ''
  busy.value = true
  try {
    const path = ban ? '/gm/account/ban' : '/gm/account/unban'
    const body = { uid: id }
    if (ban) {
      body.durationSeconds = sec
      if (reason.value.trim()) {
        body.reason = reason.value.trim()
      }
    }
    const data = await apiPost(path, body)
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

async function doMute(mute, id, sec) {
  error.value = ''
  busy.value = true
  try {
    const path = mute ? '/gm/account/mute' : '/gm/account/unmute'
    const body = { uid: id }
    if (mute) {
      body.durationSeconds = sec
      if (reason.value.trim()) {
        body.reason = reason.value.trim()
      }
    }
    const data = await apiPost(path, body)
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
