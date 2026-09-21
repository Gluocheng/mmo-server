import { apiGet } from './api.js'

const catalog = {
  bagTypes: [],
  items: [],
}

export async function loadCatalog() {
  const data = await apiGet('/gm/config/catalog')
  if (data.code && data.code !== 0) {
    throw new Error(data.message || `业务码 ${data.code}`)
  }
  catalog.bagTypes = Array.isArray(data.bagTypes) ? data.bagTypes : []
  catalog.items = Array.isArray(data.items) ? data.items : []
  return catalog
}

export function bagTypeName(id) {
  const n = Number(id)
  const hit = catalog.bagTypes.find((b) => Number(b.id) === n)
  return hit ? hit.name : n ? `类型 ${n}` : '—'
}

export function itemName(id) {
  const n = Number(id)
  const hit = catalog.items.find((it) => Number(it.id) === n)
  return hit ? hit.name : n ? `道具 ${n}` : '—'
}

export function bagTypeOptions(includeAll) {
  const opts = catalog.bagTypes.map((b) => ({
    label: b.name,
    value: Number(b.id),
  }))
  if (includeAll) {
    return [{ label: '全部背包', value: 0 }, ...opts]
  }
  return opts
}

export function itemOptions() {
  return catalog.items.map((it) => ({
    label: it.name,
    value: Number(it.id),
  }))
}

export function defaultBagType() {
  const consumable = catalog.bagTypes.find((b) => Number(b.id) === 2)
  if (consumable) {
    return 2
  }
  return catalog.bagTypes.length ? Number(catalog.bagTypes[0].id) : 0
}
