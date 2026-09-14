<template>
  <n-card title="账号管理">
    <n-space vertical>
      <n-space>
        <n-input v-model:value="form.username" placeholder="用户名" style="width: 140px" />
        <n-input v-model:value="form.password" type="password" placeholder="密码（至少 6 位）" style="width: 160px" />
        <n-input v-model:value="form.displayName" placeholder="显示名" style="width: 140px" />
        <n-select v-model:value="form.role" :options="roleOptions" style="width: 140px" />
        <n-button type="primary" :loading="creating" @click="create">创建</n-button>
      </n-space>
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-data-table :columns="columns" :data="rows" :loading="busy" />
    </n-space>
    <n-modal v-model:show="pwdShow" preset="dialog" title="重置密码" positive-text="确认" negative-text="取消" @positive-click="confirmPwd">
      <n-input v-model:value="pwdValue" type="password" show-password-on="click" :placeholder="pwdPlaceholder" />
    </n-modal>
  </n-card>
</template>

<script setup>
import { h, onMounted, reactive, ref } from 'vue'
import { NButton, NSpace, useDialog, useMessage } from 'naive-ui'
import { apiGet, apiPost } from '../api.js'

const dialog = useDialog()
const message = useMessage()
const busy = ref(false)
const creating = ref(false)
const error = ref('')
const rows = ref([])
const form = reactive({
  username: '',
  password: '',
  displayName: '',
  role: 'operator',
})
const roleOptions = [
  { label: '运营', value: 'operator' },
  { label: '管理员', value: 'admin' },
]
const pwdShow = ref(false)
const pwdValue = ref('')
const pwdUser = ref(null)
const pwdPlaceholder = ref('新密码（至少 6 位）')

const columns = [
  { title: 'ID', key: 'id', width: 70 },
  { title: '用户名', key: 'username' },
  { title: '显示名', key: 'displayName' },
  { title: '角色', key: 'role', width: 100 },
  {
    title: '禁用',
    key: 'disabled',
    width: 80,
    render(row) {
      return row.disabled ? '是' : '否'
    },
  },
  {
    title: '操作',
    key: 'ops',
    width: 200,
    render(row) {
      return h(NSpace, null, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              onClick: () => toggle(row),
            },
            { default: () => (row.disabled ? '启用' : '禁用') }
          ),
          h(
            NButton,
            {
              size: 'small',
              onClick: () => openPwd(row),
            },
            { default: () => '改密' }
          ),
        ],
      })
    },
  },
]

async function load() {
  error.value = ''
  busy.value = true
  try {
    const data = await apiGet('/gm/users')
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    rows.value = data.list || []
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    busy.value = false
  }
}

async function create() {
  error.value = ''
  creating.value = true
  try {
    const data = await apiPost('/gm/users', { ...form })
    if (data.code && data.code !== 0) {
      error.value = data.message || `业务码 ${data.code}`
      return
    }
    form.username = ''
    form.password = ''
    form.displayName = ''
    message.success('已创建')
    await load()
  } catch (e) {
    error.value = e.message || '请求失败'
  } finally {
    creating.value = false
  }
}

function toggle(row) {
  const next = !row.disabled
  dialog.warning({
    title: next ? '禁用账号' : '启用账号',
    content: `确认${next ? '禁用' : '启用'} ${row.username}？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      const data = await apiPost('/gm/users/disable', { id: row.id, disabled: next })
      if (data.code && data.code !== 0) {
        message.error(data.message || `业务码 ${data.code}`)
        return
      }
      await load()
    },
  })
}

function openPwd(row) {
  pwdUser.value = row
  pwdValue.value = ''
  pwdPlaceholder.value = `为 ${row.username} 设置新密码（至少 6 位）`
  pwdShow.value = true
}

async function confirmPwd() {
  const row = pwdUser.value
  const pwd = pwdValue.value
  if (!row || !pwd) {
    message.error('请填写新密码')
    return false
  }
  const data = await apiPost('/gm/users/password', { id: row.id, password: pwd })
  if (data.code && data.code !== 0) {
    message.error(data.message || `业务码 ${data.code}`)
    return false
  }
  message.success('已改密')
  return true
}

onMounted(load)
</script>
