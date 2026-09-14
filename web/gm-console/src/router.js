import { createRouter, createWebHistory } from 'vue-router'
import { fetchMe } from './auth.js'
import Login from './views/Login.vue'
import Shell from './views/Shell.vue'
import Health from './views/Health.vue'
import Account from './views/Account.vue'
import Player from './views/Player.vue'
import Bag from './views/Bag.vue'
import Grant from './views/Grant.vue'
import Kick from './views/Kick.vue'
import Reload from './views/Reload.vue'
import Logs from './views/Logs.vue'
import Users from './views/Users.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login, meta: { public: true } },
    {
      path: '/',
      component: Shell,
      children: [
        { path: '', component: Health },
        { path: 'account', component: Account },
        { path: 'player', component: Player },
        { path: 'bag', component: Bag },
        { path: 'grant', component: Grant },
        { path: 'kick', component: Kick },
        { path: 'reload', component: Reload },
        { path: 'logs', component: Logs },
        { path: 'users', component: Users, meta: { admin: true } },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const me = await fetchMe()
  if (to.meta.public) {
    if (me && to.path === '/login') {
      return { path: '/' }
    }
    return true
  }
  if (!me) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.admin && me.role !== 'admin') {
    return { path: '/' }
  }
  return true
})

export default router
