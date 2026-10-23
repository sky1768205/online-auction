import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { parseTime, useServerNow } from '../realtime/clock'

const pad = (n: number) => String(n).padStart(2, '0')

/** Обратный отсчёт до endsAt по серверным часам. */
export function useCountdown(endsAt: MaybeRefOrGetter<string>) {
  const now = useServerNow()

  const remainingMs = computed(() => Math.max(0, parseTime(toValue(endsAt)) - now.value))

  const text = computed(() => {
    const total = Math.floor(remainingMs.value / 1000)
    const d = Math.floor(total / 86400)
    const h = Math.floor((total % 86400) / 3600)
    const m = Math.floor((total % 3600) / 60)
    const s = total % 60
    if (d > 0) return `${d} д ${pad(h)}:${pad(m)}:${pad(s)}`
    if (h > 0) return `${pad(h)}:${pad(m)}:${pad(s)}`
    return `${pad(m)}:${pad(s)}`
  })

  return {
    remainingMs,
    text,
    expired: computed(() => remainingMs.value === 0),
    urgent: computed(() => remainingMs.value > 0 && remainingMs.value < 60_000),
  }
}
