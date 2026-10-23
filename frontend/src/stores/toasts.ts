import { defineStore } from 'pinia'

export type ToastKind = 'info' | 'success' | 'warning'

export interface Toast {
  id: number
  kind: ToastKind
  title: string
  text?: string
  to?: string // ссылка «перейти к лоту»
}

let nextId = 1

export const useToasts = defineStore('toasts', {
  state: () => ({ items: [] as Toast[] }),
  actions: {
    push(t: Omit<Toast, 'id'>, ttl = 7000) {
      const id = nextId++
      this.items.push({ ...t, id })
      if (this.items.length > 4) this.items.shift()
      setTimeout(() => this.dismiss(id), ttl)
    },
    dismiss(id: number) {
      this.items = this.items.filter((t) => t.id !== id)
    },
  },
})
