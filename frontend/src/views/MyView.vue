<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { applyBid, applyCancelled, applyClosed, type BidPlaced, type Lot, type LotCancelled, type LotClosed, type Participation } from '../api/types'
import { socket } from '../realtime/socket'
import { useAuth } from '../stores/auth'
import { formatRub, pluralBids } from '../utils/format'
import Countdown from '../components/Countdown.vue'

type Tab = 'active' | 'finished' | 'selling'

const auth = useAuth()
const route = useRoute()
const router = useRouter()

const participations = ref<Participation[]>([])
const myLots = ref<Lot[]>([])
const loading = ref(true)
const error = ref('')

const tab = computed<Tab>(() => (['finished', 'selling'].includes(route.query.tab as string) ? (route.query.tab as Tab) : 'active'))
const active = computed(() => participations.value.filter((p) => p.lot.status === 'active'))
const finished = computed(() => participations.value.filter((p) => p.lot.status !== 'active'))
const leadingCount = computed(() => active.value.filter((p) => p.lot.leaderId === auth.user?.id).length)

function state(p: Participation): { label: string; cls: string } {
  const me = auth.user?.id
  if (p.lot.status === 'active') {
    return p.lot.leaderId === me ? { label: 'Вы лидируете', cls: 'badge-success' } : { label: 'Ставку перебили', cls: 'badge-danger' }
  }
  if (p.lot.status === 'cancelled') return { label: 'Снят с торгов', cls: 'badge-danger' }
  return p.lot.winnerId === me ? { label: 'Выиграно', cls: 'badge-success' } : { label: 'Не выиграно', cls: '' }
}

function sellingState(l: Lot): { label: string; cls: string } {
  if (l.status === 'active') return { label: 'Идут торги', cls: 'badge-accent' }
  if (l.status === 'cancelled') return { label: 'Снят с торгов', cls: 'badge-danger' }
  return l.winnerId ? { label: 'Продан', cls: 'badge-success' } : { label: 'Не продан', cls: '' }
}

async function load() {
  error.value = ''
  try {
    const [bids, lots] = await Promise.all([api.myBids(), api.myLots()])
    participations.value = bids.items
    myLots.value = lots.items
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Не удалось загрузить данные'
  } finally {
    loading.value = false
  }
}

// Комната catalog получает все ставки и закрытия — этого хватает, чтобы статусы
// «лидирую / перебили / выиграл» менялись прямо на этой странице.
function onEvent(type: string, data: unknown) {
  const lots = [...participations.value.map((p) => p.lot), ...myLots.value]
  if (type === 'bid:new') {
    const e = data as BidPlaced
    lots.filter((l) => l.id === e.lotId).forEach((l) => applyBid(l, e))
    const mine = participations.value.find((p) => p.lot.id === e.lotId)
    if (mine && e.bid.userId === auth.user?.id) mine.myMaxBid = Math.max(mine.myMaxBid, e.bid.amount)
  } else if (type === 'lot:closed') {
    const e = data as LotClosed
    lots.filter((l) => l.id === e.lotId).forEach((l) => applyClosed(l, e))
  } else if (type === 'lot:cancelled') {
    const e = data as LotCancelled
    lots.filter((l) => l.id === e.lotId).forEach((l) => applyCancelled(l, e))
  }
}

let unsubscribe = () => {}
let offReconnect = () => {}
onMounted(() => {
  unsubscribe = socket.subscribe('catalog', onEvent)
  offReconnect = socket.onReconnect(load)
  load()
})
onUnmounted(() => {
  unsubscribe()
  offReconnect()
})

function setTab(t: Tab) {
  router.replace({ query: t === 'active' ? {} : { tab: t } })
}
</script>

<template>
  <div class="my">
    <div class="head">
      <div class="caps muted kicker">Личный кабинет</div>
      <h1 class="page-title">Мои торги</h1>
      <p v-if="active.length" class="muted">
        Участвуете в {{ active.length }} · лидируете в {{ leadingCount }}
      </p>
    </div>

    <div class="tabs" role="tablist">
      <button class="tab caps" :class="{ active: tab === 'active' }" @click="setTab('active')">
        Участвую <span class="count">{{ active.length }}</span>
      </button>
      <button class="tab caps" :class="{ active: tab === 'finished' }" @click="setTab('finished')">
        Завершённые <span class="count">{{ finished.length }}</span>
      </button>
      <button class="tab caps" :class="{ active: tab === 'selling' }" @click="setTab('selling')">
        Мои лоты <span class="count">{{ myLots.length }}</span>
      </button>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div v-if="loading" class="list">
      <div v-for="i in 3" :key="i" class="row skeleton-row">
        <div class="thumb skeleton" />
        <div class="info"><div class="skeleton line" /><div class="skeleton line short" /></div>
      </div>
    </div>

    <template v-else-if="tab !== 'selling'">
      <div v-if="(tab === 'active' ? active : finished).length === 0" class="empty">
        <p>{{ tab === 'active' ? 'Вы пока не участвуете в торгах.' : 'Завершённых торгов пока нет.' }}</p>
        <RouterLink to="/catalog" class="btn btn-primary">Перейти в каталог</RouterLink>
      </div>
      <TransitionGroup v-else tag="div" name="list" class="list">
        <RouterLink v-for="p in tab === 'active' ? active : finished" :key="p.lot.id" :to="`/lots/${p.lot.id}`" class="row">
          <img v-if="p.lot.coverUrl" :src="p.lot.coverUrl" class="thumb" alt="" loading="lazy" />
          <div v-else class="thumb" />
          <div class="info">
            <div class="title">{{ p.lot.title }}</div>
            <div class="meta">
              <span class="badge" :class="state(p).cls">{{ state(p).label }}</span>
              <span class="muted small">{{ pluralBids(p.lot.bidCount) }}</span>
            </div>
          </div>
          <div class="nums">
            <div class="price serif tabular">
              {{ formatRub(p.lot.status === 'active' ? p.lot.currentPrice : p.lot.finalPrice) }}
            </div>
            <div class="muted small tabular">моя: {{ formatRub(p.myMaxBid) }}</div>
            <Countdown v-if="p.lot.status === 'active'" class="small" :ends-at="p.lot.endsAt" :status="p.lot.status" />
          </div>
        </RouterLink>
      </TransitionGroup>
    </template>

    <template v-else>
      <div v-if="myLots.length === 0" class="empty">
        <p>Вы ещё ничего не выставляли.</p>
        <RouterLink to="/lots/new" class="btn btn-primary">Выставить лот</RouterLink>
      </div>
      <div v-else class="list">
        <RouterLink v-for="l in myLots" :key="l.id" :to="`/lots/${l.id}`" class="row">
          <img v-if="l.coverUrl" :src="l.coverUrl" class="thumb" alt="" loading="lazy" />
          <div v-else class="thumb" />
          <div class="info">
            <div class="title">{{ l.title }}</div>
            <div class="meta">
              <span class="badge" :class="sellingState(l).cls">{{ sellingState(l).label }}</span>
              <span class="muted small">{{ pluralBids(l.bidCount) }}</span>
            </div>
          </div>
          <div class="nums">
            <div class="price serif tabular">
              {{ formatRub(l.status === 'active' ? (l.currentPrice ?? l.startPrice) : l.finalPrice) }}
            </div>
            <div v-if="l.status !== 'active' && l.winnerName" class="muted small">{{ l.winnerName }}</div>
            <Countdown v-if="l.status === 'active'" class="small" :ends-at="l.endsAt" :status="l.status" />
          </div>
        </RouterLink>
      </div>
    </template>
  </div>
</template>

<style scoped>
.my {
  display: flex;
  flex-direction: column;
  max-width: 980px;
  margin: 0 auto;
}
.kicker {
  margin-bottom: 12px;
}
.page-title {
  font-size: clamp(34px, 4.4vw, 54px);
  letter-spacing: -0.02em;
}
.head p {
  margin: 12px 0 0;
}
.tabs {
  display: flex;
  gap: 32px;
  margin-top: 32px;
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
.alert {
  margin-top: 20px;
}
.list {
  display: flex;
  flex-direction: column;
}
.row {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 18px 0;
  border-bottom: 1px solid var(--border);
  transition: background 0.2s;
}
.row:hover .title {
  color: var(--accent);
}
.thumb {
  width: 112px;
  height: 84px;
  object-fit: cover;
  background: var(--surface-2);
  flex-shrink: 0;
}
.info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.title {
  font-size: 18px;
  font-weight: 500;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  transition: color 0.2s;
}
.meta {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}
.nums {
  text-align: right;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.price {
  font-size: 26px;
  font-weight: 600;
  line-height: 1;
  color: var(--text);
}
.small {
  font-size: 13px;
}
.empty {
  padding: 72px 20px;
  text-align: center;
}
.empty p {
  margin: 0 0 20px;
  font-size: 20px;
  color: var(--text);
}
.skeleton-row {
  pointer-events: none;
}
.skeleton-row .thumb {
  display: block;
}
.line {
  height: 14px;
}
.line.short {
  width: 40%;
}
.list-move {
  transition: transform 0.3s;
}
@media (max-width: 640px) {
  .tabs {
    gap: 20px;
    margin-top: 20px;
  }
  .row {
    gap: 12px;
    padding: 14px 0;
  }
  .thumb {
    width: 64px;
    height: 64px;
  }
  .title {
    font-size: 15px;
  }
  .price {
    font-size: 20px;
  }
}
</style>
