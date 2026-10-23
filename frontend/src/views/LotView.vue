<script setup lang="ts">
import { computed, ref, watch, watchEffect, onUnmounted, nextTick } from 'vue'
import { api } from '../api/client'
import { applyBid, applyCancelled, applyClosed, type Bid, type BidPlaced, type Lot, type LotCancelled, type LotClosed } from '../api/types'
import { socket } from '../realtime/socket'
import { useAuth } from '../stores/auth'
import { useCountdown } from '../composables/useCountdown'
import { formatDateTime, formatRub, pluralBids } from '../utils/format'
import Countdown from '../components/Countdown.vue'
import ImageGallery from '../components/ImageGallery.vue'
import BidPanel from '../components/BidPanel.vue'
import BidHistory from '../components/BidHistory.vue'
import Icon from '../components/Icon.vue'

const props = defineProps<{ id: number }>()
const auth = useAuth()

const lot = ref<Lot | null>(null)
const bids = ref<Bid[]>([])
const error = ref('')
const notice = ref('')
const watching = ref(0)
const priceKey = ref(0) // перезапускает анимацию подсветки цены

const isWinner = computed(() => lot.value?.status === 'finished' && auth.user != null && lot.value.winnerId === auth.user.id)
const isLeader = computed(() => auth.user != null && lot.value?.leaderId === auth.user.id)
const { text: timeLeft, expired } = useCountdown(() => lot.value?.endsAt ?? new Date().toISOString())

async function load() {
  try {
    const [res, history] = await Promise.all([api.lot(props.id), api.bids(props.id)])
    lot.value = res.lot
    bids.value = history
    error.value = ''
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Не удалось загрузить лот'
  }
}

// Одна точка входа для ставок: и из WebSocket, и из ответа на собственный POST
// (своё событие тоже придёт по сокету — дубли отсекаются по id ставки).
function onBid(e: BidPlaced) {
  if (!lot.value || e.lotId !== lot.value.id) return
  applyBid(lot.value, e)
  if (!bids.value.some((b) => b.id === e.bid.id)) {
    bids.value.unshift(e.bid)
    bids.value.sort((a, b) => b.id - a.id)
    priceKey.value++
  }
  if (e.extended) notice.value = 'Ставка в последние секунды — таймер продлён'
}

function onEvent(type: string, data: any) {
  if (!lot.value) return
  if (type === 'bid:new') onBid(data as BidPlaced)
  else if (type === 'lot:closed') applyClosed(lot.value, data as LotClosed)
  else if (type === 'lot:cancelled') applyCancelled(lot.value, data as LotCancelled)
  else if (type === 'presence') watching.value = data.watching
}

let unsubscribe = () => {}
const offReconnect = socket.onReconnect(load)

watch(
  () => props.id,
  (id) => {
    unsubscribe()
    lot.value = null
    bids.value = []
    notice.value = ''
    // Подписываемся до загрузки: события, пришедшие во время запроса, не потеряются.
    unsubscribe = socket.subscribe(`lot:${id}`, onEvent)
    load()
  },
  { immediate: true },
)

// Цена и таймер в заголовке вкладки — видно, даже если вкладка в фоне.
const originalTitle = document.title
watchEffect(() => {
  const l = lot.value
  if (!l) return
  document.title =
    l.status === 'active' && !expired.value
      ? `${timeLeft.value} · ${formatRub(l.currentPrice ?? l.startPrice)} — ${l.title}`
      : `${l.title} — Лот`
})

// Нижняя панель на телефоне показывается, когда форма ставки ушла с экрана.
const panelEl = ref<HTMLElement>()
const bidPanel = ref<InstanceType<typeof BidPanel>>()
const panelVisible = ref(true)
const observer = new IntersectionObserver(([entry]) => (panelVisible.value = entry.isIntersecting), { threshold: 0 })
watch(panelEl, (el, old) => {
  if (old) observer.unobserve(old)
  if (el) observer.observe(el)
})

async function scrollToBid() {
  await nextTick()
  bidPanel.value?.focus()
}

onUnmounted(() => {
  unsubscribe()
  offReconnect()
  observer.disconnect()
  document.title = originalTitle
})
</script>

<template>
  <div v-if="error" class="alert alert-error">{{ error }}</div>

  <!-- Скелетон на время загрузки -->
  <div v-else-if="!lot" class="layout">
    <div class="gallery-area skeleton sk-gallery" />
    <div class="side-area sk-side">
      <div class="skeleton sk-line" style="width: 30%" />
      <div class="skeleton sk-line big" />
      <div class="skeleton sk-line big" style="width: 70%" />
      <div class="skeleton sk-line" style="width: 50%; height: 48px; margin-top: 24px" />
      <div class="skeleton sk-line" style="height: 46px; margin-top: 24px" />
    </div>
  </div>

  <article v-else class="lot-page">
    <nav class="crumbs caps">
      <RouterLink to="/catalog">Каталог</RouterLink>
      <span class="sep" />
      <RouterLink :to="{ path: '/catalog', query: { category: lot.categorySlug } }">{{ lot.categoryName }}</RouterLink>
    </nav>

    <div class="layout">
      <div class="gallery-area">
        <ImageGallery :images="lot.images ?? []" :alt="lot.title" />
      </div>

      <aside class="side-area">
        <div class="status-row">
          <span v-if="lot.status === 'active'" class="caps status live-status"><span class="live" /> Идут торги</span>
          <span v-else-if="lot.status === 'cancelled'" class="caps status cancelled-status">Снят с торгов</span>
          <span v-else class="caps status muted">Торги завершены</span>
          <span v-if="watching > 1" class="caps muted watching" title="Сейчас на странице лота">
            <Icon name="eye" :size="16" /> {{ watching }} смотрят
          </span>
        </div>

        <h1 class="title">{{ lot.title }}</h1>
        <div class="seller muted">Продавец: {{ lot.sellerName }}</div>

        <template v-if="lot.status === 'active'">
          <div class="figures">
            <div class="figure">
              <div class="caps muted">{{ lot.currentPrice ? 'Текущая ставка' : 'Стартовая цена' }}</div>
              <div :key="priceKey" class="price serif tabular" :class="{ flash: priceKey > 0 }">
                {{ formatRub(lot.currentPrice ?? lot.startPrice) }}
              </div>
              <div class="muted small">
                <template v-if="lot.leaderName">{{ pluralBids(lot.bidCount) }} · лидер: <strong class="leader">{{ isLeader ? 'вы' : lot.leaderName }}</strong></template>
                <template v-else>Ставок пока нет</template>
              </div>
            </div>
            <div class="figure timer">
              <div class="caps muted">До окончания</div>
              <Countdown class="timer-value" :ends-at="lot.endsAt" :status="lot.status" large />
              <div class="muted small">{{ formatDateTime(lot.endsAt) }}</div>
            </div>
          </div>

          <div v-if="notice" class="alert notice"><Icon name="clock" :size="16" /> {{ notice }}</div>

          <div ref="panelEl" class="bid-area"><BidPanel ref="bidPanel" :lot="lot" @placed="onBid" /></div>
        </template>

        <template v-else-if="lot.status === 'cancelled'">
          <div class="alert alert-error cancelled">
            <Icon name="alert" :size="18" />
            <div>
              <strong>Лот снят с торгов администратором.</strong>
              <div v-if="lot.cancelReason">Причина: {{ lot.cancelReason }}</div>
              <div v-if="lot.closedAt" class="small">{{ formatDateTime(lot.closedAt) }} · ставки больше не принимаются</div>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="figures">
            <div class="figure">
              <div class="caps muted">Итоговая цена</div>
              <div class="price serif tabular">{{ lot.finalPrice ? formatRub(lot.finalPrice) : '—' }}</div>
              <div v-if="lot.closedAt" class="muted small">Торги закрыты {{ formatDateTime(lot.closedAt) }}</div>
            </div>
          </div>
          <div v-if="isWinner" class="alert alert-success result"><Icon name="trophy" :size="18" /> Поздравляем, вы выиграли этот лот!</div>
          <div v-else-if="lot.winnerName" class="result">Победитель: <strong>{{ lot.winnerName }}</strong></div>
          <div v-else class="result muted">Ставок не было — лот не продан</div>
        </template>

        <dl class="facts">
          <div><dt>Стартовая цена</dt><dd class="tabular">{{ formatRub(lot.startPrice) }}</dd></div>
          <div><dt>Шаг торгов</dt><dd class="tabular">{{ formatRub(lot.minStep) }}</dd></div>
          <div><dt>Категория</dt><dd>{{ lot.categoryName }}</dd></div>
        </dl>
      </aside>

      <section class="desc-area">
        <h2 class="block-title">Описание</h2>
        <p v-if="lot.description" class="text">{{ lot.description }}</p>
        <p v-else class="muted">Продавец не добавил описание.</p>
      </section>

      <div class="history-area">
        <BidHistory :bids="bids" :total="lot.bidCount" :open="lot.status === 'active'" />
      </div>
    </div>

    <!-- Мобильная панель «цена + ставка», закреплённая над нижней навигацией -->
    <Transition name="slide">
      <div v-if="lot.status === 'active' && !panelVisible" class="sticky-bid">
        <div class="sticky-info">
          <div class="serif tabular sticky-price">{{ formatRub(lot.currentPrice ?? lot.startPrice) }}</div>
          <Countdown class="small" :ends-at="lot.endsAt" :status="lot.status" />
        </div>
        <button v-if="isLeader" class="btn btn-sm" disabled>Вы лидируете</button>
        <button v-else class="btn btn-primary btn-sm" @click="scrollToBid">Ставка от {{ formatRub(lot.minNextBid) }}</button>
      </div>
    </Transition>
  </article>
</template>

<style scoped>
.crumbs {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 11px;
  color: var(--muted);
  margin-bottom: 28px;
}
.crumbs a:hover {
  color: var(--accent);
}
.sep {
  width: 24px;
  height: 1px;
  background: var(--border-strong);
}

/* Десктоп: галерея, описание и история слева, панель торгов справа (липкая).
   Телефон: галерея → торги → история → описание. */
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
  grid-template-areas:
    'gallery side'
    'desc side'
    'history side';
  grid-template-rows: auto auto 1fr;
  gap: 48px 64px;
  align-items: start;
}
.gallery-area {
  grid-area: gallery;
}
.desc-area {
  grid-area: desc;
}
.history-area {
  grid-area: history;
}
.side-area {
  grid-area: side;
  position: sticky;
  top: calc(var(--header-h) + 24px);
  display: flex;
  flex-direction: column;
}

.status-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  font-size: 11px;
}
.status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.live-status {
  color: var(--accent);
}
.cancelled-status {
  color: var(--danger);
}
.cancelled {
  display: flex;
  gap: 12px;
  margin-top: 28px;
  line-height: 1.5;
}
.cancelled .small {
  margin-top: 4px;
  opacity: 0.8;
}
.live {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
  animation: blink 1.5s infinite;
}
@keyframes blink {
  50% {
    opacity: 0.3;
  }
}
.watching {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.title {
  font-size: clamp(28px, 3vw, 40px);
  letter-spacing: -0.015em;
  margin-top: 18px;
}
.seller {
  margin-top: 10px;
  font-size: 14px;
}

.figures {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 24px;
  margin-top: 32px;
  padding: 24px 0;
  border-top: 1px solid var(--border-strong);
  border-bottom: 1px solid var(--border);
}
.figure .caps {
  font-size: 10.5px;
  margin-bottom: 8px;
}
.price {
  font-size: clamp(40px, 4vw, 54px);
  font-weight: 600;
  line-height: 1;
  color: var(--text);
  padding: 2px 4px;
  margin-left: -4px;
  width: fit-content;
}
.timer {
  text-align: right;
}
.timer-value {
  display: block;
  line-height: 1;
  padding: 2px 0;
}
.timer-value:deep(*),
.timer-value {
  font-family: var(--serif);
}
.small {
  font-size: 13px;
  margin-top: 8px;
}
.leader {
  color: var(--text);
  font-weight: 600;
}
.notice {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
  background: var(--accent-soft);
  color: var(--accent);
}
.bid-area {
  margin-top: 24px;
}
.result {
  margin-top: 20px;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 16px;
}
.facts {
  margin: 32px 0 0;
  font-size: 14px;
}
.facts div {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--border);
}
.facts dt {
  color: var(--muted);
}
.facts dd {
  margin: 0;
  color: var(--text);
}

.block-title {
  font-size: 26px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--border-strong);
}
.text {
  white-space: pre-line;
  margin: 0;
  font-size: 17px;
  line-height: 1.7;
  max-width: 680px;
}

.sk-gallery {
  aspect-ratio: 4 / 3;
}
.sk-side {
  gap: 14px;
}
.sk-line {
  height: 14px;
}
.sk-line.big {
  height: 34px;
}
.sticky-bid {
  display: none;
}

@media (max-width: 960px) {
  .layout {
    grid-template-columns: 1fr;
    grid-template-areas: 'gallery' 'side' 'history' 'desc';
    grid-template-rows: none;
    gap: 36px;
  }
  .side-area {
    position: static;
  }
}

@media (max-width: 640px) {
  .crumbs {
    display: none;
  }
  .layout {
    gap: 28px;
  }
  .title {
    font-size: 26px;
    margin-top: 12px;
  }
  .figures {
    margin-top: 20px;
    padding: 18px 0;
  }
  .price {
    font-size: 36px;
  }
  .timer-value {
    font-size: 30px;
  }
  .block-title {
    font-size: 22px;
  }
  .text {
    font-size: 16px;
  }
  .sticky-bid {
    position: fixed;
    left: 0;
    right: 0;
    bottom: calc(57px + env(safe-area-inset-bottom));
    z-index: 25;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 16px;
    background: var(--bg);
    border-top: 1px solid var(--border-strong);
  }
  .sticky-price {
    font-size: 24px;
    font-weight: 600;
    color: var(--text);
    line-height: 1;
  }
  .slide-enter-active,
  .slide-leave-active {
    transition: transform 0.2s ease;
  }
  .slide-enter-from,
  .slide-leave-to {
    transform: translateY(100%);
  }
}
</style>
