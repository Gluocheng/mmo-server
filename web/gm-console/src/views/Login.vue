<template>
  <div class="login-wrap">
    <n-card title="GM 控制台" style="width: 380px" :bordered="false">
      <p class="hint">使用运营账号登录。开发环境默认管理员 <code>admin</code> / <code>admin123</code>，上线后请立即改密。</p>
      <n-form @submit.prevent="submit">
        <n-form-item label="用户名">
          <n-input v-model:value="username" placeholder="用户名" autocomplete="username" @keyup.enter="submit" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            placeholder="密码"
            autocomplete="current-password"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-alert v-if="error" type="error" style="margin-bottom: 12px">{{ error }}</n-alert>
        <n-button type="primary" block :loading="busy" @click="submit">登录</n-button>
      </n-form>
    </n-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../auth.js'

const router = useRouter()
const route = useRoute()
const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (!username.value.trim() || !password.value) {
    error.value = '请填写用户名和密码'
    return
  }
  busy.value = true
  try {
    const r = await login(username.value.trim(), password.value)
    if (!r.ok) {
      error.value = r.http === 401 ? '用户名或密码错误' : r.message || '登录失败'
      return
    }
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect === '/login' ? '/' : redirect)
  } catch (e) {
    error.value = e.message || '登录失败'
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: #f3f4f6;
}
.hint {
  color: #6b7280;
  margin-top: 0;
  font-size: 13px;
}
</style>
