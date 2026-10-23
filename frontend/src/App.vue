<script setup lang="ts">
import { onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuth } from './stores/auth'
import { useToasts } from './stores/toasts'
import { useTheme } from './composables/useTheme'
import { socket } from './realtime/socket'
import type { LotCancelled, LotClosedPersonal, Outbid } from './api/types'
import { formatRub } from './utils/format'
import ToastStack from './components/ToastStack.vue'
import Icon from './components/Icon.vue'

const auth = useAuth()
const toasts = useToasts()
const router = useRouter()
const { theme, toggle: toggleTheme } = useTheme()
const connected = socket.connected

// Персональные уведомления приходят на любой странице.
const off = socket.onPersonal((type, data) => {
  if (type === 'bid:outbid') {
    const e = data as Outbid
    toasts.push({
      kind: 'warning',
      title: 'Вашу ставку перебили',
      text: `«${e.lotTitle}»: ${e.leaderName} поставил(а) ${formatRub(e.currentPrice)}`,
      to: `/lots/${e.lotId}`,
    })
  } else if (type === 'lot:won') {
    const e = data as LotClosedPersonal
    toasts.push({ kind: 'success', title: 'Вы выиграли аукцион!', text: `«${e.lotTitle}» за ${formatRub(e.finalPrice)}`, to: `/lots/${e.lotId}` }, 15000)
  } else if (type === 'lot:cancelled') {
    const e = data as LotCancelled
    const mine = e.sellerId === auth.user?.id
    toasts.push({
      kind: 'warning',
      title: mine ? 'Ваш лот снят с торгов' : 'Лот, где вы лидировали, снят с торгов',
      text: `«${e.lotTitle}». Причина: ${e.reason}`,
      to: `/lots/${e.lotId}`,
    }, 15000)
  } else if (type === 'lot:sold') {
    const e = data as LotClosedPersonal
    toasts.push({
      kind: 'info',
      title: e.winnerName ? 'Ваш лот продан' : 'Торги по вашему лоту завершены',
      text: e.winnerName ? `«${e.lotTitle}» — ${formatRub(e.finalPrice)}, покупатель ${e.winnerName}` : `«${e.lotTitle}» — ставок не было`,
      to: `/lots/${e.lotId}`,
    }, 15000)
  }
})
onUnmounted(off)

async function logout() {
  await auth.logout()
  router.push('/')
}
</script>

<template>
  <header class="header">
    <div class="header-inner">
      <nav class="nav desktop-only">
        <RouterLink to="/catalog" class="nav-link" active-class="active">Каталог</RouterLink>
        <RouterLink to="/#how" class="nav-link hide-lg">Как это работает</RouterLink>
        <RouterLink v-if="auth.user" to="/me" class="nav-link" active-class="active">Мои торги</RouterLink>
        <RouterLink v-if="auth.user?.role === 'admin'" to="/admin" class="nav-link admin-link" active-class="active">
          <Icon name="shield" :size="15" /> <span class="hide-md">Админ-панель</span>
        </RouterLink>
      </nav>

      <RouterLink to="/" class="logo" aria-label="Лот — на главную">
        <span class="logo-mark serif">Лот</span>
        <span class="logo-sub">аукционный дом</span>
      </RouterLink>

      <div class="actions">
        <RouterLink v-if="auth.user" to="/lots/new" class="nav-link desktop-only sell">
          <Icon name="plus" :size="16" /> <span class="hide-md">Выставить лот</span>
        </RouterLink>
        <RouterLink v-if="auth.user?.role === 'admin'" to="/admin" class="icon-btn mobile-only" title="Админ-панель" aria-label="Админ-панель">
          <Icon name="shield" />
        </RouterLink>
        <button class="icon-btn" :title="theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'" aria-label="Переключить тему" @click="toggleTheme">
          <Icon :name="theme === 'dark' ? 'sun' : 'moon'" />
        </button>
        <template v-if="auth.user">
          <RouterLink to="/me" class="user desktop-only" :title="connected ? 'Онлайн' : 'Переподключение…'">
            <Icon name="user" :size="18" />
            <span class="hide-md">{{ auth.user.displayName }}</span>
            <span class="conn" :class="{ on: connected }" />
          </RouterLink>
          <button class="icon-btn" title="Выйти" aria-label="Выйти" @click="logout"><Icon name="logout" /></button>
        </template>
        <RouterLink v-else-if="auth.loaded" to="/login" class="nav-link login">
          <Icon name="user" :size="18" /> <span class="desktop-only">Войти</span>
        </RouterLink>
      </div>
    </div>
  </header>

  <main class="main">
    <RouterView v-slot="{ Component, route }">
      <!-- Главная — во всю ширину (полноэкранный слайдер), остальные страницы в контейнере -->
      <component :is="Component" v-if="route.meta.fullWidth" />
      <div v-else class="container page">
        <component :is="Component" />
      </div>
    </RouterView>
  </main>

  <footer class="footer">
    <div class="container footer-inner">
      <div class="footer-brand">
        <div class="logo-mark serif">Лот</div>
        <p>Онлайн-аукцион искусства, антиквариата и предметов коллекционирования. Ставки в реальном времени.</p>
      </div>
      <div class="footer-col">
        <div class="caps">Покупателям</div>
        <RouterLink to="/catalog">Каталог лотов</RouterLink>
        <RouterLink to="/#how">Как это работает</RouterLink>
        <RouterLink to="/#faq">Правила торгов</RouterLink>
      </div>
      <div class="footer-col">
        <div class="caps">Продавцам</div>
        <RouterLink to="/lots/new">Выставить лот</RouterLink>
        <RouterLink to="/me?tab=selling">Мои лоты</RouterLink>
      </div>
      <div class="footer-col">
        <div class="caps">Аккаунт</div>
        <RouterLink v-if="auth.user" to="/me">Мои торги</RouterLink>
        <template v-else>
          <RouterLink to="/login">Войти</RouterLink>
          <RouterLink to="/register">Регистрация</RouterLink>
        </template>
      </div>
    </div>
    <div class="container footer-bottom">
      <span>© {{ new Date().getFullYear() }} Лот. Учебный проект.</span>
      <button class="theme-switch" @click="toggleTheme">
        <Icon :name="theme === 'dark' ? 'sun' : 'moon'" :size="16" />
        {{ theme === 'dark' ? 'Светлая тема' : 'Тёмная тема' }}
      </button>
    </div>
  </footer>

  <!-- Нижняя навигация на телефоне -->
  <nav class="tabbar">
    <RouterLink to="/" class="tab" exact-active-class="active"><Icon name="home" />Главная</RouterLink>
    <RouterLink to="/catalog" class="tab" active-class="active"><Icon name="grid" />Каталог</RouterLink>
    <RouterLink :to="auth.user ? '/lots/new' : '/login?redirect=/lots/new'" class="tab" active-class="active"><Icon name="plus" />Продать</RouterLink>
    <RouterLink :to="auth.user ? '/me' : '/login?redirect=/me'" class="tab" active-class="active"><Icon name="gavel" />Мои торги</RouterLink>
  </nav>

  <ToastStack />
</template>

<style scoped>
.header {
  position: sticky;
  top: 0;
  z-index: 40;
  background: color-mix(in srgb, var(--bg) 92%, transparent);
  backdrop-filter: blur(14px);
  border-bottom: 1px solid var(--border);
}
.header-inner {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  height: var(--header-h);
  padding: 0 32px;
  max-width: 1440px;
  margin: 0 auto;
}
.nav {
  display: flex;
  gap: 28px;
}
.nav-link {
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--text);
  padding: 6px 0;
  border-bottom: 1px solid transparent;
  transition: color 0.2s, border-color 0.2s;
}
.nav-link:hover {
  color: var(--accent);
}
.nav-link.active {
  border-bottom-color: var(--accent);
}
.logo {
  display: flex;
  flex-direction: column;
  align-items: center;
  line-height: 1;
  color: var(--text);
}
.logo-mark {
  font-size: 34px;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.logo-sub {
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.28em;
  text-transform: uppercase;
  color: var(--muted);
  margin-top: 2px;
}
.actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}
.sell {
  margin-right: 12px;
}
.user {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
  padding: 0 8px;
}
.user:hover {
  color: var(--accent);
}
.conn {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--danger);
}
.conn.on {
  background: var(--success);
}
.main {
  min-height: calc(100vh - var(--header-h) - 200px);
}
.page {
  padding-top: 40px;
  padding-bottom: 80px;
}

.footer {
  background: var(--inverse-bg);
  color: color-mix(in srgb, var(--inverse-text) 70%, transparent);
  padding: 64px 0 0;
  margin-top: 40px;
  font-size: 14px;
}
.footer-inner {
  display: grid;
  grid-template-columns: 2fr 1fr 1fr 1fr;
  gap: 40px;
}
.footer-brand .logo-mark {
  color: var(--inverse-text);
}
.footer-brand p {
  max-width: 340px;
  margin: 12px 0 0;
}
.footer-col {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.footer-col .caps {
  color: var(--inverse-text);
  margin-bottom: 4px;
}
.footer-col a:hover {
  color: var(--accent);
}
.footer-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin-top: 56px;
  padding-top: 20px;
  padding-bottom: 24px;
  border-top: 1px solid color-mix(in srgb, var(--inverse-text) 15%, transparent);
  font-size: 13px;
}
.theme-switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: none;
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.theme-switch:hover {
  color: var(--accent);
}
.tabbar {
  display: none;
}
.mobile-only {
  display: none;
}
.admin-link {
  color: var(--accent);
}

/* Средние экраны: прячем второстепенные подписи, чтобы меню не наезжало на логотип */
@media (max-width: 1180px) {
  .hide-lg {
    display: none !important;
  }
  .nav {
    gap: 20px;
  }
}
@media (max-width: 960px) {
  .hide-md {
    display: none !important;
  }
  .sell {
    margin-right: 0;
  }
}

@media (max-width: 900px) {
  .footer-inner {
    grid-template-columns: 1fr 1fr;
  }
  .footer-brand {
    grid-column: 1 / -1;
  }
}

@media (max-width: 640px) {
  .desktop-only {
    display: none !important;
  }
  .mobile-only {
    display: inline-grid;
  }
  .header-inner {
    grid-template-columns: auto 1fr;
    padding: 0 8px 0 16px;
  }
  .logo {
    align-items: flex-start;
  }
  .logo-mark {
    font-size: 28px;
  }
  .logo-sub {
    font-size: 8px;
  }
  .login {
    padding: 0 10px;
  }
  .page {
    padding-top: 20px;
    padding-bottom: 40px;
  }
  .footer {
    padding-top: 40px;
    /* место под нижнюю навигацию */
    padding-bottom: calc(64px + env(safe-area-inset-bottom));
  }
  .footer-bottom {
    flex-direction: column;
    align-items: flex-start;
    margin-top: 32px;
  }
  .tabbar {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 30;
    padding-bottom: env(safe-area-inset-bottom);
    background: color-mix(in srgb, var(--bg) 94%, transparent);
    backdrop-filter: blur(14px);
    border-top: 1px solid var(--border);
  }
  .tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    padding: 9px 0 7px;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .tab.active {
    color: var(--accent);
  }
}
</style>
