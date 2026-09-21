<template>
  <n-layout has-sider style="height: 100vh">
    <n-layout-sider bordered collapse-mode="width" :collapsed-width="64" :width="220" show-trigger>
      <div class="brand">GM 控制台</div>
      <n-menu
        :value="active"
        :options="menuOptions"
        :collapsed-width="64"
        :collapsed-icon-size="20"
        @update:value="onMenu"
      />
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered style="height: 56px; padding: 0 20px; display: flex; align-items: center; justify-content: space-between">
        <span>{{ userLabel }}</span>
        <n-button quaternary @click="onLogout">退出</n-button>
      </n-layout-header>
      <n-layout-content content-style="padding: 20px 24px 32px; background: #f5f7fa;">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup>
import { computed, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import {
  BagOutline,
  FlashOutline,
  GlobeOutline,
  GridOutline,
  HourglassOutline,
  LogOutOutline,
  MegaphoneOutline,
  PeopleOutline,
  PersonOutline,
  PulseOutline,
  RemoveCircleOutline,
  SearchOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import { currentUser, isAdmin, logout } from '../auth.js'

const router = useRouter()
const route = useRoute()
const user = currentUser()
const userLabel = user ? `操作者 ${user.displayName}（${user.role}）` : ''

function renderIcon(icon) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions = computed(() => {
  const items = [
    { label: '概览', key: '/', icon: renderIcon(PulseOutline) },
    { label: '查账号', key: '/account', icon: renderIcon(SearchOutline) },
    { label: '查角色', key: '/player', icon: renderIcon(PersonOutline) },
    { label: '查背包', key: '/bag', icon: renderIcon(BagOutline) },
    { label: '发道具', key: '/grant', icon: renderIcon(GridOutline) },
    { label: '扣道具', key: '/deduct', icon: renderIcon(RemoveCircleOutline) },
    { label: '踢下线', key: '/kick', icon: renderIcon(LogOutOutline) },
    { label: '在线列表', key: '/online', icon: renderIcon(GlobeOutline) },
    { label: '公告', key: '/notice', icon: renderIcon(MegaphoneOutline) },
    { label: '游戏时间', key: '/time', icon: renderIcon(HourglassOutline) },
    { label: '配表热更', key: '/reload', icon: renderIcon(FlashOutline) },
    { label: '操作记录', key: '/logs', icon: renderIcon(TimeOutline) },
  ]
  if (isAdmin()) {
    items.push({ label: '账号管理', key: '/users', icon: renderIcon(PeopleOutline) })
  }
  return items
})

const active = computed(() => (route.path === '' ? '/' : route.path))

function onMenu(key) {
  router.push(key)
}

async function onLogout() {
  await logout()
  router.replace('/login')
}
</script>

<style scoped>
.brand {
  height: 56px;
  display: flex;
  align-items: center;
  padding: 0 20px;
  font-weight: 600;
}
</style>
