import { ref } from 'vue'
import { calibrate } from './clock'

type Handler = (type: string, data: any) => void

/**
 * Единственное WebSocket-соединение на вкладку.
 *
 * - Компоненты подписываются на комнаты ("catalog", "lot:42"); подписки
 *   считаются по ссылкам — комната покидается, когда уходит последний обработчик.
 * - При обрыве — переподключение с экспоненциальной задержкой и повторная
 *   подписка на все комнаты.
 * - Пока соединения не было, события могли потеряться, поэтому после
 *   переподключения вызываются onReconnect-обработчики (они перезагружают
 *   состояние через REST).
 * - Персональные события (комната "user:<id>") сервер шлёт сам авторизованному
 *   клиенту; после входа/выхода соединение пересоздаётся через reconnect().
 */
class RealtimeSocket {
  readonly connected = ref(false)

  private ws: WebSocket | null = null
  private rooms = new Map<string, Set<Handler>>()
  private reconnectHandlers = new Set<() => void>()
  private personalHandlers = new Set<Handler>()
  private attempt = 0
  private wasConnected = false
  private pingTimer: ReturnType<typeof setInterval> | undefined
  private pingSentAt = 0

  connect() {
    if (this.ws) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/ws`)
    this.ws = ws

    ws.onopen = () => {
      this.connected.value = true
      this.attempt = 0
      for (const room of this.rooms.keys()) this.send({ type: 'subscribe', room })
      if (this.wasConnected) this.reconnectHandlers.forEach((fn) => fn())
      this.wasConnected = true
      this.ping()
      this.pingTimer = setInterval(() => this.ping(), 30_000)
    }

    ws.onmessage = (e) => {
      let msg: { type: string; room?: string; data?: any }
      try {
        msg = JSON.parse(e.data)
      } catch {
        return
      }
      if (msg.type === 'pong') {
        calibrate(msg.data.serverTime, this.pingSentAt, Date.now())
        return
      }
      if (msg.room?.startsWith('user:')) this.personalHandlers.forEach((h) => h(msg.type, msg.data))
      else if (msg.room) this.rooms.get(msg.room)?.forEach((h) => h(msg.type, msg.data))
    }

    ws.onclose = () => {
      if (this.ws !== ws) return // соединение закрыто намеренно через reconnect()
      this.ws = null
      this.connected.value = false
      clearInterval(this.pingTimer)
      const delay = Math.min(1000 * 2 ** this.attempt, 15_000) + Math.random() * 500
      this.attempt++
      setTimeout(() => this.connect(), delay)
    }
  }

  /** Подписка на комнату; возвращает функцию отписки. */
  subscribe(room: string, handler: Handler): () => void {
    let handlers = this.rooms.get(room)
    if (!handlers) {
      handlers = new Set()
      this.rooms.set(room, handlers)
      this.send({ type: 'subscribe', room })
    }
    handlers.add(handler)

    return () => {
      handlers.delete(handler)
      if (handlers.size === 0 && this.rooms.get(room) === handlers) {
        this.rooms.delete(room)
        this.send({ type: 'unsubscribe', room })
      }
    }
  }

  /** Персональные уведомления: bid:outbid, lot:won, lot:sold. */
  onPersonal(fn: Handler): () => void {
    this.personalHandlers.add(fn)
    return () => this.personalHandlers.delete(fn)
  }

  /** Пересоздаёт соединение — нужно после входа или выхода, чтобы сервер узнал пользователя. */
  reconnect() {
    const old = this.ws
    this.ws = null
    clearInterval(this.pingTimer)
    old?.close()
    this.connect()
  }

  onReconnect(fn: () => void): () => void {
    this.reconnectHandlers.add(fn)
    return () => this.reconnectHandlers.delete(fn)
  }

  private ping() {
    this.pingSentAt = Date.now()
    this.send({ type: 'ping' })
  }

  private send(msg: object) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(msg))
  }
}

export const socket = new RealtimeSocket()
