import { ref } from 'vue'

export type Theme = 'light' | 'dark'

const KEY = 'theme'
const media = window.matchMedia('(prefers-color-scheme: dark)')

function saved(): Theme | null {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'dark' ? v : null
  } catch {
    return null
  }
}

// Начальное значение уже выставил скрипт в index.html.
const theme = ref<Theme>((document.documentElement.dataset.theme as Theme) ?? 'light')

function apply(t: Theme) {
  theme.value = t
  document.documentElement.dataset.theme = t
}

// Пока пользователь не выбрал тему сам — следуем настройке системы.
media.addEventListener('change', (e) => {
  if (!saved()) apply(e.matches ? 'dark' : 'light')
})

export function useTheme() {
  return {
    theme,
    toggle() {
      const next: Theme = theme.value === 'dark' ? 'light' : 'dark'
      apply(next)
      try {
        localStorage.setItem(KEY, next)
      } catch {
        /* приватный режим — тема просто не запомнится */
      }
    },
  }
}
