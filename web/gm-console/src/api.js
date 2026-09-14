import { clearMe } from './auth.js'

export async function apiFetch(path, options = {}) {
  const headers = { ...(options.headers || {}) }
  if (options.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json'
  }
  const res = await fetch(path, { credentials: 'include', ...options, headers })
  const data = await res.json().catch(() => ({
    code: -1,
    message: `HTTP ${res.status}`,
  }))
  if (res.status === 401) {
    clearMe()
    if (window.location.pathname !== '/login') {
      window.location.assign('/login')
    }
  }
  data._http = res.status
  return data
}

export function apiGet(path) {
  return apiFetch(path, { method: 'GET' })
}

export function apiPost(path, body) {
  return apiFetch(path, { method: 'POST', body: JSON.stringify(body) })
}

export function formatResult(data) {
  if (!data) {
    return ''
  }
  const copy = { ...data }
  delete copy._http
  return JSON.stringify(copy, null, 2)
}

export function formatUnix(sec) {
  if (!sec) {
    return ''
  }
  const d = new Date(sec * 1000)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
