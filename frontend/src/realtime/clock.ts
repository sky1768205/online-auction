import { readonly, ref } from 'vue'

/**
 * Синхронизация с часами сервера.
 *
 * Таймер обратного отсчёта считается от серверного времени, иначе у пользователя
 * со сбитыми часами торги «закончились бы» раньше или позже. Смещение оценивается
 * по ответам сервера: offset = serverTime − (отправка + получение) / 2.
 * Берём образец с наименьшим RTT из последних — он точнее всего.
 */
const samples: { rtt: number; offset: number }[] = []
const offset = ref(0)

export function parseTime(iso: string): number {
  // Go отдаёт наносекунды — оставляем миллисекунды для надёжного разбора во всех браузерах.
  return Date.parse(iso.replace(/(\.\d{3})\d+/, '$1'))
}

export function calibrate(serverTimeIso: string, sentAt: number, receivedAt: number) {
  const server = parseTime(serverTimeIso)
  if (Number.isNaN(server)) return
  samples.push({ rtt: receivedAt - sentAt, offset: server - (sentAt + receivedAt) / 2 })
  if (samples.length > 10) samples.shift()
  offset.value = samples.reduce((best, s) => (s.rtt < best.rtt ? s : best)).offset
}

export function serverNow(): number {
  return Date.now() + offset.value
}

// Один общий «тик» на всё приложение вместо интервала в каждом таймере.
const now = ref(serverNow())
setInterval(() => (now.value = serverNow()), 250)

export function useServerNow() {
  return readonly(now)
}
