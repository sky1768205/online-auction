import { defineStore } from 'pinia'
import { api } from '../api/client'
import type { User } from '../api/types'
import { socket } from '../realtime/socket'

export const useAuth = defineStore('auth', {
  state: () => ({
    user: null as User | null,
    loaded: false,
  }),
  actions: {
    async load() {
      try {
        this.user = await api.me()
      } catch {
        this.user = null
      } finally {
        this.loaded = true
      }
    },
    async login(email: string, password: string) {
      this.user = await api.login(email, password)
      socket.reconnect()
    },
    async loginDemo(role: string) {
      this.user = await api.demoLogin(role)
      socket.reconnect()
    },
    async register(email: string, password: string, displayName: string) {
      this.user = await api.register(email, password, displayName)
      socket.reconnect()
    },
    async logout() {
      await api.logout()
      this.user = null
      socket.reconnect()
    },
  },
})
