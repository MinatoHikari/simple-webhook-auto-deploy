import { reactive } from 'vue'

export const toasts = reactive([])

let id = 0

export function toast(message, type = 'info') {
  const t = { id: ++id, message, type }
  toasts.push(t)
  setTimeout(() => {
    const i = toasts.indexOf(t)
    if (i >= 0) toasts.splice(i, 1)
  }, 2800)
}
