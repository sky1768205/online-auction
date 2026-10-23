<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import {
  applyBid,
  applyCancelled,
  applyClosed,
  type AdminStats,
  type AdminUser,
  type BidPlaced,
  type FeedBid,
  type Lot,
  type LotCancelled,
  type LotClosed,
} from '../api/types'
import { socket } from '../realtime/socket'
import { useAuth } from '../stores/auth'
import { useToasts } from '../stores/toasts'
import { formatDateTime, formatRub, plural } from '../utils/format'
import Countdown from '../components/Countdown.vue'
import Icon from '../components/Icon.vue'

type Tab = 'overview' | 'lots' | 'users'

const route = useRoute()
const router = useRouter()
const auth = useAuth()
const toasts = useToasts()

const tab = computed<Tab>(() => (['lots', 'users'].includes(route.query.tab as string) ? (route.query.tab as Tab) : 'overview'))
function setTab(t: Tab) {
  router.replace({ query: t === 'overview' ? {} : { tab: t } })
}

// ---------- Обзор ----------
const stats = ref<AdminStats | null>(null)
const feed = ref<FeedBid[]>([])

async function loadOverview() {
  const [s, b] = await Promise.all([api.admin.stats(), api.admin.bids()])
  stats.value = s
  feed.value = b
}

const tiles = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { label: 'Идут торги', value: String(s.activeLots), hint: `${plural(s.finishedLots, 'завершён', 'завершено', 'завершено')}, ${s.cancelledLots} снято` },
    { label: 'Ставок за сутки', value: String(s.bids24h), hint: `всего ${s.bids}` },
    { label: 'Оборот', value: formatRub(s.turnover), hint: `${plural(s.soldLots, 'лот продан', 'лота продано', 'лотов продано')}` },
    { label: 'Онлайн сейчас', value: String(s.online), hint: 'WebSocket-подключений' },
    { label: 'Пользователи', value: String(s.users), hint: s.blockedUsers ? `${s.blockedUsers} заблокировано` : 'без блокировок' },
  ]
})

// ---------- Лоты ----------
const lots = ref<Lot[]>([])
const lotsTotal = ref(0)
const lotStatus = ref('')
const lotQuery = ref('')
const lotsLoading = ref(false)

async function loadLots() {
  lotsLoading.value = true
  try {
    const res = await api.admin.lots({ status: lotStatus.value, q: lotQuery.value })
    lots.value = res.items
    lotsTotal.value = res.total
  } finally {
    lotsLoading.value = false
  }
}

const statusFilters = [
  { value: '', label: 'Все' },
  { value: 'active', label: 'Идут торги' },
  { value: 'finished', label: 'Завершены' },
  { value: 'cancelled', label: 'Сняты' },
]

function lotBadge(l: Lot) {
  if (l.status === 'active') return { label: 'Идут торги', cls: 'badge-accent' }
  if (l.status === 'cancelled') return { label: 'Снят', cls: 'badge-danger' }
  return l.winnerId ? { label: 'Продан', cls: 'badge-success' } : { label: 'Не продан', cls: '' }
}

// ---------- Пользователи ----------
const users = ref<AdminUser[]>([])
const userQuery = ref('')

async function loadUsers() {
  users.value = await api.admin.users(userQuery.value)
}

// ---------- Поиск с задержкой ----------
let timer: ReturnType<typeof setTimeout>
watch([lotQuery, lotStatus], () => {
  clearTimeout(timer)
  timer = setTimeout(loadLots, 250)
})
watch(userQuery, () => {
  clearTimeout(timer)
  timer = setTimeout(loadUsers, 250)
})

// ---------- Модальные окна ----------
const reasons = ['Нарушение правил площадки', 'Запрещённый товар', 'Подозрение на подделку', 'По просьбе продавца']
const cancelTarget = ref<Lot | null>(null)
const cancelReason = ref('')
const blockTarget = ref<AdminUser | null>(null)
const blockCancelLots = ref(true)
const busy = ref(false)
const modalError = ref('')

function openCancel(l: Lot) {
  cancelTarget.value = l
  cancelReason.value = ''
  modalError.value = ''
}
function openBlock(u: AdminUser) {
  blockTarget.value = u
  blockCancelLots.value = u.activeLots > 0
  modalError.value = ''
}
function closeModals() {
  cancelTarget.value = null
  blockTarget.value = null
}

async function confirmCancel() {
  if (!cancelTarget.value) return
  busy.value = true
  modalError.value = ''
  try {
    const c = await api.admin.cancelLot(cancelTarget.value.id, cancelReason.value)
    toasts.push({ kind: 'info', title: 'Лот снят с торгов', text: `«${c.lotTitle}»` })
    closeModals()
    refreshAll()
  } catch (e) {
    modalError.value = e instanceof Error ? e.message : 'Ошибка'
  } finally {
    busy.value = false
  }
}

async function confirmBlock() {
  if (!blockTarget.value) return
  busy.value = true
  modalError.value = ''
  try {
    const name = blockTarget.value.displayName
    const res = await api.admin.blockUser(blockTarget.value.id, blockCancelLots.value)
    toasts.push({
      kind: 'info',
      title: `${name} заблокирован(а)`,
      text: res.cancelledLots ? `Снято лотов: ${res.cancelledLots}` : undefined,
    })
    closeModals()
    refreshAll()
  } catch (e) {
    modalError.value = e instanceof Error ? e.message : 'Ошибка'
  } finally {
    busy.value = false
  }
}

async function unblock(u: AdminUser) {
  await api.admin.unblockUser(u.id)
  toasts.push({ kind: 'success', title: `${u.displayName} разблокирован(а)` })
  loadUsers()
  loadOverview()
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') closeModals()
}

// ---------- Живые обновления ----------
function refreshAll() {
  loadOverview()
  loadLots()
  loadUsers()
}

let statsTimer: ReturnType<typeof setInterval>
function onEvent(type: string, data: unknown) {
  if (type === 'bid:new') {
    const e = data as BidPlaced
    feed.value = [{ ...e.bid, lotTitle: e.lotTitle }, ...feed.value.filter((b) => b.id !== e.bid.id)].slice(0, 30)
    if (stats.value) {
      stats.value.bids++
      stats.value.bids24h++
    }
    const l = lots.value.find((x) => x.id === e.lotId)
    if (l) applyBid(l, e)
  } else if (type === 'lot:closed') {
    const l = lots.value.find((x) => x.id === (data as LotClosed).lotId)
    if (l) applyClosed(l, data as LotClosed)
    loadOverview()
  } else if (type === 'lot:cancelled') {
    const l = lots.value.find((x) => x.id === (data as LotCancelled).lotId)
    if (l) applyCancelled(l, data as LotCancelled)
  } else if (type === 'lot:created') {
    loadLots()
    loadOverview()
  }
}

let unsubscribe = () => {}
let offReconnect = () => {}
onMounted(() => {
  refreshAll()
  unsubscribe = socket.subscribe('catalog', onEvent)
  offReconnect = socket.onReconnect(refreshAll)
  statsTimer = setInterval(() => api.admin.stats().then((s) => (stats.value = s)).catch(() => {}), 15000)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  unsubscribe()
  offReconnect()
  clearInterval(statsTimer)
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="admin">
    <header class="head">
      <div>
        <div class="caps muted kicker"><Icon name="shield" :size="14" /> Администрирование</div>
        <h1 class="page-title">Админ-панель</h1>
      </div>
      <span class="caps live"><span class="dot" /> Обновляется в реальном времени</span>
    </header>

    <nav class="tabs" role="tablist">
      <button class="tab caps" :class="{ active: tab === 'overview' }" @click="setTab('overview')">Обзор</button>
      <button class="tab caps" :class="{ active: tab === 'lots' }" @click="setTab('lots')">
        Лоты <span class="count">{{ lotsTotal }}</span>
      </button>
      <button class="tab caps" :class="{ active: tab === 'users' }" @click="setTab('users')">
        Пользователи <span class="count">{{ users.length }}</span>
      </button>
    </nav>

    <!-- ===== Обзор ===== -->
    <section v-if="tab === 'overview'" class="overview">
      <div class="tiles">
        <template v-if="!stats">
          <div v-for="i in 5" :key="i" class="tile skeleton" />
        </template>
        <div v-for="t in tiles" v-else :key="t.label" class="tile">
          <div class="caps muted">{{ t.label }}</div>
          <div class="tile-value serif tabular">{{ t.value }}</div>
          <div class="muted small">{{ t.hint }}</div>
        </div>
      </div>

      <div class="feed">
        <h2 class="block-title">Лента ставок</h2>
        <p v-if="!feed.length" class="muted">Ставок пока нет.</p>
        <TransitionGroup v-else tag="ol" name="feed" class="feed-list">
          <li v-for="b in feed" :key="b.id">
            <span class="feed-time muted tabular">{{ formatDateTime(b.createdAt) }}</span>
            <span class="feed-who">{{ b.userName }}</span>
            <RouterLink :to="`/lots/${b.lotId}`" class="feed-lot">{{ b.lotTitle }}</RouterLink>
            <span class="feed-amount serif tabular">{{ formatRub(b.amount) }}</span>
          </li>
        </TransitionGroup>
      </div>
    </section>

    <!-- ===== Лоты ===== -->
    <section v-else-if="tab === 'lots'">
      <div class="toolbar">
        <label class="search">
          <Icon name="search" :size="18" />
          <input v-model="lotQuery" class="input input-line" type="search" placeholder="Поиск по названию" />
        </label>
        <div class="chips">
          <button v-for="f in statusFilters" :key="f.value" class="chip" :class="{ active: lotStatus === f.value }" @click="lotStatus = f.value">
            #{{ f.label }}
          </button>
        </div>
      </div>

      <div class="table" :class="{ loading: lotsLoading }">
        <div class="row header caps muted">
          <span>Лот</span><span>Статус</span><span>Цена</span><span>Ставки</span><span>Окончание</span><span />
        </div>
        <div v-for="l in lots" :key="l.id" class="row">
          <RouterLink :to="`/lots/${l.id}`" class="lot-cell">
            <img v-if="l.coverUrl" :src="l.coverUrl" class="thumb" alt="" loading="lazy" />
            <span v-else class="thumb" />
            <span class="lot-info">
              <span class="lot-title">{{ l.title }}</span>
              <span class="muted small">{{ l.sellerName }} · {{ l.categoryName }}</span>
              <span v-if="l.cancelReason" class="small danger">{{ l.cancelReason }}</span>
            </span>
          </RouterLink>
          <span><span class="badge" :class="lotBadge(l).cls">{{ lotBadge(l).label }}</span></span>
          <span class="serif tabular price" data-label="Цена">
            {{ formatRub(l.status === 'active' ? (l.currentPrice ?? l.startPrice) : l.finalPrice) }}
          </span>
          <span class="tabular" data-label="Ставки">{{ l.bidCount }}</span>
          <span class="small tabular" data-label="Окончание">
            <Countdown v-if="l.status === 'active'" :ends-at="l.endsAt" :status="l.status" />
            <template v-else-if="l.closedAt">{{ formatDateTime(l.closedAt) }}</template>
          </span>
          <span class="actions">
            <button v-if="l.status === 'active'" class="btn btn-sm danger-btn" @click="openCancel(l)">Снять</button>
          </span>
        </div>
        <p v-if="!lotsLoading && !lots.length" class="muted empty">Лоты не найдены.</p>
      </div>
    </section>

    <!-- ===== Пользователи ===== -->
    <section v-else>
      <div class="toolbar">
        <label class="search">
          <Icon name="search" :size="18" />
          <input v-model="userQuery" class="input input-line" type="search" placeholder="Имя или email" />
        </label>
      </div>

      <div class="table users">
        <div class="row header caps muted">
          <span>Пользователь</span><span>Статус</span><span>Лоты</span><span>Ставки</span><span>Победы</span><span />
        </div>
        <div v-for="u in users" :key="u.id" class="row" :class="{ blocked: u.blockedAt }">
          <span class="user-cell">
            <span class="avatar serif">{{ u.displayName.slice(0, 1) }}</span>
            <span class="lot-info">
              <span class="lot-title">
                {{ u.displayName }}
                <span v-if="u.id === auth.user?.id" class="muted small">(вы)</span>
              </span>
              <span class="muted small">{{ u.email }}</span>
            </span>
          </span>
          <span>
            <span v-if="u.role === 'admin'" class="badge badge-accent">Админ</span>
            <span v-else-if="u.blockedAt" class="badge badge-danger">Заблокирован</span>
            <span v-else class="badge badge-success">Активен</span>
          </span>
          <span class="tabular" data-label="Лоты">{{ u.lots }}<span v-if="u.activeLots" class="muted small"> · {{ u.activeLots }} акт.</span></span>
          <span class="tabular" data-label="Ставки">{{ u.bids }}</span>
          <span class="tabular" data-label="Победы">{{ u.wins }}</span>
          <span class="actions">
            <template v-if="u.role !== 'admin'">
              <button v-if="u.blockedAt" class="btn btn-sm" @click="unblock(u)">Разблокировать</button>
              <button v-else class="btn btn-sm danger-btn" @click="openBlock(u)">Заблокировать</button>
            </template>
          </span>
        </div>
      </div>
    </section>

    <!-- ===== Модальные окна ===== -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="cancelTarget || blockTarget" class="overlay" @click.self="closeModals">
          <form v-if="cancelTarget" class="modal" @submit.prevent="confirmCancel">
            <div class="caps muted">Снятие с торгов</div>
            <h2>«{{ cancelTarget.title }}»</h2>
            <p class="muted">
              Торги остановятся сразу: ставки перестанут приниматься, продавец и лидер торгов получат уведомление.
              Отменить действие нельзя.
            </p>
            <label class="field">
              <span>Причина — её увидят участники</span>
              <textarea v-model="cancelReason" class="textarea" rows="3" required minlength="3" maxlength="500" />
            </label>
            <div class="presets">
              <button v-for="r in reasons" :key="r" type="button" class="chip" @click="cancelReason = r">#{{ r }}</button>
            </div>
            <div v-if="modalError" class="alert alert-error">{{ modalError }}</div>
            <div class="modal-actions">
              <button type="button" class="btn" @click="closeModals">Отмена</button>
              <button class="btn btn-primary danger-fill" :disabled="busy || cancelReason.trim().length < 3">Снять с торгов</button>
            </div>
          </form>

          <form v-else-if="blockTarget" class="modal" @submit.prevent="confirmBlock">
            <div class="caps muted">Блокировка пользователя</div>
            <h2>{{ blockTarget.displayName }}</h2>
            <p class="muted">
              Пользователь не сможет войти, делать ставки и выставлять лоты. Блокировку можно снять в любой момент.
            </p>
            <label v-if="blockTarget.activeLots" class="check">
              <input v-model="blockCancelLots" type="checkbox" />
              <span>Снять с торгов активные лоты пользователя ({{ blockTarget.activeLots }})</span>
            </label>
            <div v-if="modalError" class="alert alert-error">{{ modalError }}</div>
            <div class="modal-actions">
              <button type="button" class="btn" @click="closeModals">Отмена</button>
              <button class="btn btn-primary danger-fill" :disabled="busy">Заблокировать</button>
            </div>
          </form>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.admin {
  display: flex;
  flex-direction: column;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  flex-wrap: wrap;
}
.kicker {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
  color: var(--accent);
}
.page-title {
  font-size: clamp(34px, 4.4vw, 54px);
  letter-spacing: -0.02em;
}
.live {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 10.5px;
  color: var(--muted);
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--success);
  animation: blink 1.5s infinite;
}
@keyframes blink {
  50% {
    opacity: 0.3;
  }
}
.tabs {
  display: flex;
  gap: 32px;
  margin: 28px 0 32px;
  border-bottom: 1px solid var(--border-strong);
  overflow-x: auto;
  scrollbar-width: none;
}
.tab {
  flex-shrink: 0;
  border: none;
  background: none;
  padding: 12px 0;
  font-family: inherit;
  color: var(--muted);
  border-bottom: 1px solid transparent;
  margin-bottom: -1px;
  cursor: pointer;
}
.tab:hover {
  color: var(--text);
}
.tab.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.count {
  font-family: var(--serif);
  font-size: 15px;
  letter-spacing: 0;
  color: var(--accent);
  margin-left: 6px;
}
.small {
  font-size: 12.5px;
}
.danger {
  color: var(--danger);
}

/* ----- Обзор ----- */
.tiles {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  border-top: 1px solid var(--border-strong);
  border-bottom: 1px solid var(--border);
}
.tile {
  padding: 24px 20px 24px 0;
  min-height: 140px;
}
.tile + .tile {
  padding-left: 20px;
  border-left: 1px solid var(--border);
}
.tile .caps {
  font-size: 10.5px;
}
.tile-value {
  font-size: clamp(32px, 3vw, 44px);
  font-weight: 600;
  color: var(--text);
  line-height: 1;
  margin: 14px 0 8px;
  white-space: nowrap;
}
.feed {
  margin-top: 48px;
}
.block-title {
  font-size: 26px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--border-strong);
}
.feed-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.feed-list li {
  display: grid;
  grid-template-columns: 150px 140px 1fr auto;
  gap: 16px;
  align-items: baseline;
  padding: 12px 0;
  border-bottom: 1px solid var(--border);
}
.feed-time {
  font-size: 13px;
}
.feed-who {
  font-weight: 600;
  color: var(--text);
}
.feed-lot {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feed-lot:hover {
  color: var(--accent);
}
.feed-amount {
  font-size: 20px;
  font-weight: 600;
  color: var(--text);
}
.feed-enter-active {
  transition: all 0.5s ease;
}
.feed-enter-from {
  opacity: 0;
  background: var(--accent-soft);
  transform: translateY(-8px);
}

/* ----- Таблицы ----- */
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 24px;
  flex-wrap: wrap;
  margin-bottom: 24px;
}
.search {
  flex: 1;
  max-width: 420px;
  min-width: 240px;
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--muted);
  border-bottom: 1px solid var(--border-strong);
}
.search:focus-within {
  border-bottom-color: var(--accent);
}
.search .input {
  border: none;
  box-shadow: none;
  height: 44px;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 20px;
}
.chip {
  border: none;
  background: none;
  padding: 2px 0;
  font: inherit;
  font-size: 15px;
  color: var(--muted);
  border-bottom: 1px solid transparent;
  cursor: pointer;
}
.chip:hover {
  color: var(--text);
}
.chip.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
}
.table {
  border-top: 1px solid var(--border-strong);
  transition: opacity 0.2s;
}
.table.loading {
  opacity: 0.5;
}
.row {
  display: grid;
  grid-template-columns: minmax(0, 3fr) 130px 130px 80px 150px 150px;
  gap: 16px;
  align-items: center;
  padding: 14px 0;
  border-bottom: 1px solid var(--border);
}
.users .row {
  grid-template-columns: minmax(0, 3fr) 140px 110px 90px 90px 170px;
}
.row.header {
  font-size: 10.5px;
  padding: 12px 0;
}
.row.blocked .user-cell {
  opacity: 0.55;
}
.lot-cell,
.user-cell {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}
.lot-cell:hover .lot-title {
  color: var(--accent);
}
.thumb {
  width: 64px;
  height: 48px;
  object-fit: cover;
  background: var(--surface-2);
  flex-shrink: 0;
}
.avatar {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  flex-shrink: 0;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 22px;
  font-weight: 600;
}
.lot-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.lot-title {
  font-weight: 500;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  transition: color 0.2s;
}
.price {
  font-size: 19px;
  font-weight: 600;
  color: var(--text);
}
.actions {
  display: flex;
  justify-content: flex-end;
}
.danger-btn {
  color: var(--danger);
  border-color: color-mix(in srgb, var(--danger) 45%, transparent);
}
.danger-btn:hover:not(:disabled) {
  border-color: var(--danger);
  background: var(--danger-soft);
}
.empty {
  padding: 40px 0;
  text-align: center;
}

/* ----- Модальные окна ----- */
.overlay {
  position: fixed;
  inset: 0;
  z-index: 150;
  display: grid;
  place-items: center;
  padding: 16px;
  background: rgb(0 0 0 / 0.55);
  backdrop-filter: blur(3px);
}
.modal {
  width: min(520px, 100%);
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 32px;
  background: var(--surface);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-float);
}
.modal h2 {
  font-size: 26px;
}
.modal p {
  margin: 0;
  font-size: 14px;
}
.presets {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
}
.presets .chip {
  font-size: 14px;
}
.check {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  font-size: 14px;
  color: var(--text);
  cursor: pointer;
}
.check input {
  margin-top: 3px;
  accent-color: var(--accent);
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 8px;
}
.danger-fill {
  background: var(--danger);
  border-color: var(--danger);
  color: #fff;
}
.danger-fill:hover:not(:disabled) {
  background: color-mix(in srgb, var(--danger) 85%, black);
  border-color: color-mix(in srgb, var(--danger) 85%, black);
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ----- Адаптив: таблицы превращаются в карточки ----- */
@media (max-width: 1000px) {
  .tiles {
    grid-template-columns: repeat(3, 1fr);
  }
  .tile:nth-child(4) {
    padding-left: 0;
    border-left: none;
  }
  .tile:nth-child(n + 4) {
    border-top: 1px solid var(--border);
  }
}

@media (max-width: 860px) {
  .row,
  .users .row {
    grid-template-columns: 1fr auto;
    gap: 8px 16px;
  }
  .row.header {
    display: none;
  }
  .lot-cell,
  .user-cell {
    grid-column: 1 / -1;
  }
  .row > [data-label]::before {
    content: attr(data-label) ': ';
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--muted);
    font-family: var(--font);
    font-weight: 600;
  }
  .actions {
    grid-column: 1 / -1;
    justify-content: flex-start;
  }
  .feed-list li {
    grid-template-columns: 1fr auto;
    gap: 2px 12px;
  }
  .feed-time {
    grid-column: 1 / -1;
  }
}

@media (max-width: 640px) {
  .tabs {
    gap: 20px;
    margin: 16px 0 20px;
  }
  .tiles {
    grid-template-columns: 1fr 1fr;
  }
  .tile,
  .tile + .tile {
    padding: 16px 12px 16px 0;
    border-left: none;
    min-height: 0;
  }
  .tile:nth-child(even) {
    padding-left: 12px;
    border-left: 1px solid var(--border);
  }
  .tile:nth-child(n + 3) {
    border-top: 1px solid var(--border);
  }
  .tile-value {
    font-size: 28px;
  }
  .live {
    display: none;
  }
  .modal {
    padding: 24px 20px;
  }
}
</style>
