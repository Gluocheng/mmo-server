let me = null

export function currentUser() {
  return me
}

export function isAdmin() {
  return me && me.role === 'admin'
}

export function clearMe() {
  me = null
}

export async function fetchMe() {
  const res = await fetch('/gm/auth/me', { credentials: 'include' })
  if (res.status === 401) {
    me = null
    return null
  }
  const data = await res.json().catch(() => null)
  if (!data || data.code !== 0) {
    me = null
    return null
  }
  me = {
    username: data.username,
    role: data.role,
    displayName: data.displayName || data.username,
  }
  return me
}

export async function login(username, password) {
  const res = await fetch('/gm/auth/login', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const data = await res.json().catch(() => ({ code: -1, message: `HTTP ${res.status}` }))
  if (res.ok && data.code === 0) {
    me = {
      username: data.username,
      role: data.role,
      displayName: data.displayName || data.username,
    }
    return { ok: true, me }
  }
  return { ok: false, message: data.message || '登录失败', http: res.status }
}

export async function logout() {
  await fetch('/gm/auth/logout', { method: 'POST', credentials: 'include' })
  me = null
}
