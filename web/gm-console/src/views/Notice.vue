<template>
  <n-card title="全服 / 场景公告">
    <p>sceneId 为 0 或留空表示全服。在线玩家收到 onNotice，不跳过任何人。</p>
    <n-space vertical>
      <n-input v-model:value="sceneId" placeholder="sceneId，空=全服" />
      <n-input
        v-model:value="text"
        type="textarea"
        placeholder="公告内容"
        :autosize="{ minRows: 3, maxRows: 8 }"
      />
      <n-button type="primary" :loading="busy" @click="submit">发送</n-button>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <pre v-if="result" class="result-pre">{{ result }}</pre>
    </n-space>
  </n-card>
</template>

<script setup>
import { ref } from 'vue'
import { useDialog } from 'naive-ui'
import { apiPost, formatResult } from '../api.js'

const dialog = useDialog()
const sceneId = ref('')
const text = ref('')
const busy = ref(false)
const error = ref('')
const result = ref('')

function submit() {
  error.value = ''
  const bodyText = text.value.trim()
  if (!bodyText) {
    error.value = '公告内容不能为空'
    return
  }
  const sid = sceneId.value.trim() === '' ? 0 : Number(sceneId.value)
  const scope = sid ? `场景 ${sid}` : '全服'
  dialog.warning({
    title: '确认发送公告',
    content: `向${scope}发送：${bodyText}`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: () => doSend(sid, bodyText),
  })
}

async function doSend(sid, bodyText) {
  busy.value = true
  try {
    const data = await apiPost('/gm/notice', { sceneId: sid, text: bodyText })
    result.value = formatResult(data)
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
