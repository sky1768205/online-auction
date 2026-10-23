<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuth } from '../stores/auth'
import { images } from '../utils/images'
import { api } from '../api/client'
import type { DemoAccount } from '../api/types'
import Icon from '../components/Icon.vue'

const props = defineProps<{ mode: 'login' | 'register' }>()
const auth = useAuth()
const route = useRoute()
const router = useRouter()

const email = ref('')
const password = ref('')
const displayName = ref('')
const error = ref('')
const sending = ref(false)

// Быстрый вход в демо-аккаунты (бэкенд отдаёт список, только если DEMO_LOGIN=true).
const demoAccounts = ref<DemoAccount[]>([])
const demoLoading = ref('')
onMounted(async () => {
  demoAccounts.value = await api.demoAccounts().catch(() => [])
})

function goAfterLogin(fallback: string) {
  const redirect = route.query.redirect
  router.replace(typeof redirect === 'string' && redirect.startsWith('/') ? redirect : fallback)
}

async function loginDemo(acc: DemoAccount) {
  error.value = ''
  demoLoading.value = acc.role
  try {
    await auth.loginDemo(acc.role)
    // Покупателя — к его ставкам, продавца — к его лотам, администратора — в админ-панель.
    goAfterLogin({ buyer: '/me', seller: '/me?tab=selling', admin: '/admin' }[acc.role])
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Ошибка'
  } finally {
    demoLoading.value = ''
  }
}

async function submit() {
  error.value = ''
  sending.value = true
  try {
    if (props.mode === 'login') await auth.login(email.value, password.value)
    else await auth.register(email.value, password.value, displayName.value)
    goAfterLogin('/')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Ошибка'
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <aside class="art" :style="{ backgroundImage: `url(${images.auth})` }">
      <div class="art-shade" />
      <div class="art-text">
        <div class="caps">Аукционный дом «Лот»</div>
        <p class="quote serif">Каждая вещь ждёт своего ценителя</p>
      </div>
    </aside>

    <form class="auth" @submit.prevent="submit">
      <div class="caps muted">{{ mode === 'login' ? 'С возвращением' : 'Новый участник' }}</div>
      <h1>{{ mode === 'login' ? 'Вход' : 'Регистрация' }}</h1>

      <label v-if="mode === 'register'" class="field">
        <span>Имя — его увидят другие участники</span>
        <input v-model="displayName" class="input" required minlength="2" maxlength="50" autocomplete="nickname" />
      </label>
      <label class="field">
        <span>Email</span>
        <input v-model="email" class="input" type="email" required autocomplete="email" />
      </label>
      <label class="field">
        <span>Пароль{{ mode === 'register' ? ' — от 8 символов' : '' }}</span>
        <input
          v-model="password"
          class="input"
          type="password"
          required
          :minlength="mode === 'register' ? 8 : undefined"
          :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
        />
      </label>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <button class="btn btn-primary submit" :disabled="sending">
        {{ mode === 'login' ? 'Войти' : 'Создать аккаунт' }}
      </button>

      <section v-if="demoAccounts.length" class="demo">
        <div class="demo-head">
          <span class="caps">Быстрый вход</span>
          <span class="muted demo-note">демо-аккаунты</span>
        </div>
        <button
          v-for="acc in demoAccounts"
          :key="acc.role"
          type="button"
          class="demo-card"
          :disabled="!!demoLoading"
          @click="loginDemo(acc)"
        >
          <span class="demo-icon"><Icon :name="{ buyer: 'user', seller: 'gavel', admin: 'shield' }[acc.role]" /></span>
          <span class="demo-body">
            <span class="demo-title">{{ acc.title }} <span class="muted">· {{ acc.displayName }}</span></span>
            <span class="demo-desc muted">{{ acc.description }}</span>
          </span>
          <span class="demo-go">
            <span v-if="demoLoading === acc.role" class="caps">Вход…</span>
            <Icon v-else name="arrowRight" :size="18" />
          </span>
        </button>
      </section>

      <p class="switch muted">
        <template v-if="mode === 'login'">
          Нет аккаунта? <RouterLink :to="{ name: 'register', query: route.query }">Зарегистрируйтесь</RouterLink>
        </template>
        <template v-else>
          Уже есть аккаунт? <RouterLink :to="{ name: 'login', query: route.query }">Войдите</RouterLink>
        </template>
      </p>
    </form>
  </div>
</template>

<style scoped>
.auth-page {
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 600px;
  border: 1px solid var(--border);
}
.art {
  position: relative;
  background: #1a1815 center / cover no-repeat;
  color: #fff;
}
.art-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(0deg, rgb(0 0 0 / 0.7), rgb(0 0 0 / 0.1) 60%);
}
.art-text {
  position: absolute;
  left: 40px;
  right: 40px;
  bottom: 40px;
}
.art-text .caps {
  color: rgb(255 255 255 / 0.75);
  font-size: 11px;
}
.quote {
  margin: 12px 0 0;
  font-size: 36px;
  line-height: 1.15;
  font-weight: 500;
}
.auth {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 20px;
  padding: 56px clamp(24px, 5vw, 72px);
  background: var(--surface);
}
.auth .caps {
  font-size: 11px;
}
h1 {
  font-size: 42px;
  margin: -8px 0 8px;
}
.submit {
  margin-top: 8px;
}
.demo {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 8px;
  padding-top: 24px;
  border-top: 1px solid var(--border);
}
.demo-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.demo-head .caps {
  color: var(--text);
}
.demo-note {
  font-size: 13px;
}
.demo-card {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
  padding: 14px 16px;
  border: 1px solid var(--border-strong);
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
}
.demo-card:hover:not(:disabled) {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.demo-card:disabled {
  opacity: 0.6;
  cursor: progress;
}
.demo-icon {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  background: var(--surface-2);
  color: var(--accent);
}
.demo-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}
.demo-title {
  font-weight: 600;
}
.demo-desc {
  font-size: 13px;
  line-height: 1.4;
}
.demo-go {
  color: var(--accent);
  flex-shrink: 0;
}
.demo-go .caps {
  font-size: 10px;
}
.switch {
  margin: 0;
  font-size: 14px;
}
.switch a {
  color: var(--accent);
  border-bottom: 1px solid currentColor;
}

@media (max-width: 800px) {
  .auth-page {
    grid-template-columns: 1fr;
    border: none;
    min-height: 0;
  }
  .art {
    display: none;
  }
  .auth {
    padding: 8px 0 24px;
    background: none;
  }
  h1 {
    font-size: 34px;
  }
}
</style>
