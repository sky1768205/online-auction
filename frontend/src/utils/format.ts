const rub0 = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0 })
const rub2 = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', minimumFractionDigits: 2 })

/** Копейки → «12 500 ₽». */
export function formatRub(kopecks: number | null | undefined): string {
  if (kopecks == null) return '—'
  return (kopecks % 100 === 0 ? rub0 : rub2).format(kopecks / 100)
}

/** Рубли (как ввёл пользователь, можно с копейками) → копейки. */
export function rubToKop(rub: number): number {
  return Math.round(rub * 100)
}

const dateTime = new Intl.DateTimeFormat('ru-RU', {
  day: 'numeric',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

export function formatDateTime(iso: string): string {
  return dateTime.format(new Date(iso.replace(/(\.\d{3})\d+/, '$1')))
}

/** plural(5, 'лот', 'лота', 'лотов') → «5 лотов» */
export function plural(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10
  const mod100 = n % 100
  if (mod10 === 1 && mod100 !== 11) return `${n} ${one}`
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return `${n} ${few}`
  return `${n} ${many}`
}

export const pluralBids = (n: number) => plural(n, 'ставка', 'ставки', 'ставок')
