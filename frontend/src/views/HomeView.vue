<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api/client'
import { applyBid, applyClosed, type BidPlaced, type Category, type Lot, type LotCancelled, type LotClosed } from '../api/types'
import { socket } from '../realtime/socket'
import { useAuth } from '../stores/auth'
import { formatRub, plural } from '../utils/format'
import Countdown from '../components/Countdown.vue'
import LotCard from '../components/LotCard.vue'
import Icon from '../components/Icon.vue'
import { categoryImage, images } from '../utils/images'

const auth = useAuth()

const featured = ref<Lot[]>([])
const ending = ref<Lot[]>([])
const categories = ref<Category[]>([])
const stats = ref({ active: 0, finished: 0 })
const loaded = ref(false)
const lotsWord = computed(() => plural(stats.value.active, 'лот', 'лота', 'лотов').split(' ')[1])

// ---------- Слайдер ----------
// Первый слайд — о сайте, следующие — самые популярные лоты прямо сейчас.
type Slide = { kind: 'intro' } | { kind: 'lot'; lot: Lot }
const slides = computed<Slide[]>(() => [{ kind: 'intro' }, ...featured.value.map((lot) => ({ kind: 'lot' as const, lot }))])
const slide = ref(0)
let autoplay: ReturnType<typeof setInterval> | undefined

function go(delta: number) {
  const n = slides.value.length
  slide.value = (slide.value + delta + n) % n
  restartAutoplay()
}
function pauseAutoplay() {
  clearInterval(autoplay)
}
function restartAutoplay() {
  clearInterval(autoplay)
  autoplay = setInterval(() => (slide.value = (slide.value + 1) % slides.value.length), 7000)
}

// Свайп на телефоне
let touchX = 0
function onTouchStart(e: TouchEvent) {
  touchX = e.touches[0].clientX
}
function onTouchEnd(e: TouchEvent) {
  const dx = e.changedTouches[0].clientX - touchX
  if (Math.abs(dx) > 50) go(dx < 0 ? 1 : -1)
}

/** Для фона слайдера берём ту же фотографию в большем разрешении (picsum отдаёт любой размер по seed). */
function hiRes(url: string | null | undefined) {
  return url ? url.replace(/\/900\/675$/, '/1920/1080') : ''
}
const introImage = images.hero

// ---------- Данные ----------
async function load() {
  const [top, soon, cats, active, finished] = await Promise.all([
    api.lots({ sort: 'popular', limit: 4 }),
    api.lots({ sort: 'ending', limit: 6 }),
    api.categories(),
    api.lots({ status: 'active', limit: 1 }),
    api.lots({ status: 'finished', limit: 1 }),
  ]).catch(() => [null, null, [], null, null] as const)
  featured.value = top?.items ?? []
  ending.value = soon?.items ?? []
  categories.value = cats ? [...cats] : []
  stats.value = { active: active?.total ?? 0, finished: finished?.total ?? 0 }
  loaded.value = true
}

// Цены на главной тоже живые — через общую комнату catalog.
function onEvent(type: string, data: unknown) {
  const all = [...featured.value, ...ending.value]
  if (type === 'bid:new') {
    const e = data as BidPlaced
    all.filter((l) => l.id === e.lotId).forEach((l) => applyBid(l, e))
  } else if (type === 'lot:closed') {
    const e = data as LotClosed
    all.filter((l) => l.id === e.lotId).forEach((l) => applyClosed(l, e))
  } else if (type === 'lot:cancelled') {
    const id = (data as LotCancelled).lotId
    featured.value = featured.value.filter((l) => l.id !== id)
    ending.value = ending.value.filter((l) => l.id !== id)
  } else if (type === 'lot:created') {
    stats.value.active++
  }
}

let unsubscribe = () => {}
let offReconnect = () => {}
onMounted(() => {
  load()
  restartAutoplay()
  unsubscribe = socket.subscribe('catalog', onEvent)
  offReconnect = socket.onReconnect(load)
})
onUnmounted(() => {
  clearInterval(autoplay)
  unsubscribe()
  offReconnect()
})

// ---------- Контент ----------
const steps = [
  {
    title: 'Зарегистрируйтесь',
    text: 'Достаточно email и имени — оно будет видно другим участникам в истории ставок.',
  },
  {
    title: 'Выберите лот',
    text: 'В каталоге — фильтры по категориям, поиск и сортировка. На странице лота: фото, описание, стартовая цена и шаг торгов.',
  },
  {
    title: 'Делайте ставки',
    text: 'Ставка должна быть не меньше текущей цены плюс шаг. Все участники видят новую ставку мгновенно — без перезагрузки страницы.',
  },
  {
    title: 'Выигрывайте',
    text: 'Когда таймер дойдёт до нуля, торги закроются автоматически. Побеждает самая высокая ставка — победитель и продавец получают уведомление.',
  },
]

const faq = [
  {
    q: 'Что такое шаг торгов?',
    a: 'Минимальная сумма, на которую новая ставка должна превышать текущую. Например, при цене 10 000 ₽ и шаге 500 ₽ следующая ставка — от 10 500 ₽. Первая ставка может быть равна стартовой цене.',
  },
  {
    q: 'Что будет, если кто-то сделает ставку в последние секунды?',
    a: 'Таймер продлится на 30 секунд. Так у остальных участников всегда есть время ответить, и выигрывает тот, кто готов заплатить больше, а не тот, кто нажал последним.',
  },
  {
    q: 'Можно ли отменить ставку?',
    a: 'Нет — ставка является обязательством. Поэтому перед отправкой мы просим подтвердить сумму вторым нажатием.',
  },
  {
    q: 'Как я узнаю, что мою ставку перебили?',
    a: 'Придёт уведомление на любой странице сайта, а в разделе «Мои торги» статус лота сменится на «Ставку перебили».',
  },
  {
    q: 'Что если два человека поставят одновременно?',
    a: 'Сервер обрабатывает ставки по одному лоту строго по очереди. Первая принятая ставка поднимает цену, вторая получит отказ с новой минимальной суммой — ошибки и «двух победителей» не бывает.',
  },
  {
    q: 'Как выставить свой лот?',
    a: 'Нажмите «Выставить лот», добавьте фото, описание, стартовую цену, шаг торгов и длительность — от 5 минут до 7 дней. Лот сразу появится в каталоге.',
  },
]

</script>

<template>
  <!-- ===== Слайдер ===== -->
  <section class="hero" @mouseenter="pauseAutoplay" @mouseleave="restartAutoplay" @touchstart.passive="onTouchStart" @touchend="onTouchEnd">
    <TransitionGroup name="slide">
      <template v-for="(s, i) in slides" :key="s.kind === 'lot' ? s.lot.id : 'intro'">
        <div v-if="i === slide" class="hero-slide">
          <img
            v-if="s.kind === 'lot' ? s.lot.coverUrl : introImage"
            class="hero-bg"
            :src="s.kind === 'lot' ? hiRes(s.lot.coverUrl) : introImage"
            alt=""
          />
          <div class="hero-shade" />

          <div class="hero-content container">
            <template v-if="s.kind === 'intro'">
              <div class="caps hero-kicker">Онлайн-аукцион</div>
              <h1 class="display hero-title">Аукционный дом<br />в вашем браузере</h1>
              <p class="hero-text">
                Искусство, антиквариат и предметы коллекционирования. Каждая ставка видна всем участникам
                в&nbsp;момент, когда она сделана.
              </p>
              <div class="hero-actions">
                <RouterLink to="/catalog" class="btn btn-primary">Смотреть лоты</RouterLink>
                <a href="#how" class="btn btn-ghost-light">Как это работает</a>
              </div>
            </template>

            <template v-else>
              <div class="caps hero-kicker">
                {{ s.lot.status === 'active' ? 'Идут торги' : 'Торги завершены' }} · {{ s.lot.categoryName }}
              </div>
              <h2 class="display hero-title">{{ s.lot.title }}</h2>
              <div class="hero-meta serif">
                <span>{{ formatRub(s.lot.currentPrice ?? s.lot.startPrice) }}</span>
                <span class="hero-line" />
                <Countdown :ends-at="s.lot.endsAt" :status="s.lot.status" />
              </div>
              <div class="hero-actions">
                <RouterLink :to="`/lots/${s.lot.id}`" class="btn btn-primary">Сделать ставку</RouterLink>
              </div>
            </template>
          </div>
        </div>
      </template>
    </TransitionGroup>

    <div v-if="slides.length > 1" class="hero-controls">
      <button class="arrow" aria-label="Предыдущий слайд" @click="go(-1)"><span class="arrow-line left" /></button>
      <span class="counter tabular">{{ slide + 1 }} / {{ slides.length }}</span>
      <button class="arrow" aria-label="Следующий слайд" @click="go(1)"><span class="arrow-line right" /></button>
    </div>
  </section>

  <!-- ===== О сайте + цифры ===== -->
  <section class="container about">
    <p class="about-lead">
      <span class="serif accent-word">Лот</span> — площадка, где вещи с историей находят новых владельцев. Продавец выставляет
      лот со стартовой ценой, покупатели повышают ставки, а когда время выходит — побеждает самое высокое предложение.
    </p>
    <dl class="stats">
      <div>
        <dt class="serif tabular">{{ stats.active }}</dt>
        <dd class="caps">{{ lotsWord }} на торгах</dd>
      </div>
      <div>
        <dt class="serif tabular">{{ stats.finished }}</dt>
        <dd class="caps">торгов завершено</dd>
      </div>
      <div>
        <dt class="serif">24/7</dt>
        <dd class="caps">ставки онлайн</dd>
      </div>
    </dl>
  </section>

  <!-- ===== Как это работает ===== -->
  <section id="how" class="section section-tinted">
    <div class="container">
      <div class="section-head">
        <h2 class="section-title">Как это работает</h2>
        <p class="muted section-sub">Четыре шага от регистрации до победы</p>
      </div>
      <ol class="steps">
        <li v-for="(s, i) in steps" :key="s.title" class="step">
          <span class="step-num serif">{{ String(i + 1).padStart(2, '0') }}</span>
          <h3>{{ s.title }}</h3>
          <p>{{ s.text }}</p>
        </li>
      </ol>
      <div class="rules">
        <div class="rule"><Icon name="bolt" /><span><strong>В реальном времени.</strong> Новые ставки, таймер и итоги приходят мгновенно.</span></div>
        <div class="rule"><Icon name="clock" /><span><strong>Защита от «снайперов».</strong> Ставка в последние 30 секунд продлевает торги.</span></div>
        <div class="rule"><Icon name="shield" /><span><strong>Честная очередь.</strong> Одновременные ставки обрабатываются строго по порядку.</span></div>
      </div>
    </div>
  </section>

  <!-- ===== Скоро завершатся ===== -->
  <section class="section container">
    <div class="section-head row">
      <h2 class="section-title">Скоро завершатся</h2>
      <RouterLink to="/catalog" class="link-arrow caps">Все лоты <Icon name="arrowRight" :size="18" /></RouterLink>
    </div>
    <div class="tiles">
      <template v-if="!loaded">
        <div v-for="i in 6" :key="i" class="skeleton tile-skeleton" />
      </template>
      <LotCard v-for="lot in ending" v-else :key="lot.id" :lot="lot" />
    </div>
    <p v-if="loaded && ending.length === 0" class="muted">Сейчас нет активных торгов — загляните позже или выставьте свой лот.</p>
  </section>

  <!-- ===== Категории ===== -->
  <section class="section container">
    <div class="section-head row">
      <h2 class="section-title">Категории</h2>
    </div>
    <div class="categories">
      <RouterLink v-for="c in categories" :key="c.id" :to="{ path: '/catalog', query: { category: c.slug } }" class="category">
        <img :src="categoryImage(c.slug)" alt="" loading="lazy" />
        <span class="category-shade" />
        <span class="category-name">{{ c.name }}</span>
        <Icon name="arrowRight" class="category-arrow" />
      </RouterLink>
    </div>
  </section>

  <!-- ===== Продавцам ===== -->
  <section class="section container">
    <div class="sell">
      <img class="sell-img" :src="images.seller" alt="" loading="lazy" />
      <div class="sell-body">
        <div class="caps muted">Продавцам</div>
        <h2 class="section-title">Продайте вещь с историей</h2>
        <p>
          Выставить лот можно за пару минут: фотографии, описание, стартовая цена и шаг торгов. Вы видите каждую ставку
          в реальном времени и получаете уведомление, когда торги завершатся.
        </p>
        <RouterLink :to="auth.user ? '/lots/new' : '/register?redirect=/lots/new'" class="btn btn-primary">Выставить лот</RouterLink>
      </div>
    </div>
  </section>

  <!-- ===== Вопросы и ответы ===== -->
  <section id="faq" class="section container faq-section">
    <div class="section-head">
      <h2 class="section-title">Правила торгов</h2>
      <p class="muted section-sub">Ответы на частые вопросы</p>
    </div>
    <div class="faq">
      <details v-for="item in faq" :key="item.q" class="faq-item">
        <summary>
          {{ item.q }}
          <Icon name="plus" class="faq-icon" />
        </summary>
        <p>{{ item.a }}</p>
      </details>
    </div>
  </section>
</template>

<style scoped>
/* ===== Слайдер ===== */
.hero {
  position: relative;
  height: min(calc(100svh - var(--header-h)), 760px);
  min-height: 520px;
  overflow: hidden;
  background: #1a1815;
  color: #fff;
}
.hero-slide {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
}
.hero-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  animation: kenburns 14s ease-out both;
}
@keyframes kenburns {
  from {
    transform: scale(1.08);
  }
  to {
    transform: scale(1);
  }
}
.hero-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(90deg, rgb(0 0 0 / 0.72) 0%, rgb(0 0 0 / 0.35) 55%, rgb(0 0 0 / 0.15) 100%);
}
.hero-content {
  position: relative;
  width: 100%;
  padding-bottom: 60px;
}
.hero-kicker {
  color: rgb(255 255 255 / 0.8);
  margin-bottom: 20px;
}
.hero-title {
  color: #fff;
  max-width: 820px;
}
.hero-text {
  max-width: 520px;
  margin: 24px 0 0;
  font-size: 18px;
  color: rgb(255 255 255 / 0.85);
}
.hero-meta {
  display: flex;
  align-items: center;
  gap: 18px;
  margin-top: 24px;
  font-size: 30px;
  color: #fff;
}
.hero-meta :deep(.countdown) {
  font-family: var(--serif);
  font-weight: 600;
}
.hero-line {
  width: 72px;
  height: 1px;
  background: rgb(255 255 255 / 0.6);
}
.hero-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 36px;
}
.hero-controls {
  position: absolute;
  left: 50%;
  bottom: 28px;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 24px;
  color: #fff;
  font-size: 16px;
}
.arrow {
  width: 64px;
  height: 32px;
  border: none;
  background: none;
  cursor: pointer;
  position: relative;
}
.arrow-line {
  position: absolute;
  top: 50%;
  left: 0;
  right: 0;
  height: 1px;
  background: rgb(255 255 255 / 0.7);
  transition: background 0.2s, transform 0.2s;
}
.arrow-line::before {
  content: '';
  position: absolute;
  top: -4px;
  width: 9px;
  height: 9px;
  border: solid rgb(255 255 255 / 0.7);
  border-width: 1px 1px 0 0;
}
.arrow-line.left::before {
  left: 0;
  transform: rotate(-135deg);
}
.arrow-line.right::before {
  right: 0;
  transform: rotate(45deg);
}
.arrow:hover .arrow-line.left {
  transform: translateX(-6px);
}
.arrow:hover .arrow-line.right {
  transform: translateX(6px);
}
.slide-enter-active,
.slide-leave-active {
  transition: opacity 0.9s ease;
}
.slide-enter-from,
.slide-leave-to {
  opacity: 0;
}

/* ===== О сайте ===== */
.about {
  display: grid;
  grid-template-columns: 1.4fr 1fr;
  gap: 64px;
  align-items: center;
  padding-top: 96px;
  padding-bottom: 96px;
}
.about-lead {
  margin: 0;
  font-size: clamp(20px, 2.1vw, 27px);
  line-height: 1.45;
  color: var(--text);
}
.accent-word {
  color: var(--accent);
  font-size: 1.25em;
  font-weight: 600;
}
.stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 24px;
  margin: 0;
}
.stats dt {
  font-size: clamp(40px, 4.4vw, 60px);
  font-weight: 600;
  line-height: 1;
  color: var(--text);
}
.stats dd {
  margin: 10px 0 0;
  color: var(--muted);
  font-size: 11px;
}

/* ===== Секции ===== */
.section {
  padding-top: 88px;
  padding-bottom: 88px;
  scroll-margin-top: var(--header-h);
}
.section-tinted {
  background: var(--surface-2);
}
.section-head {
  margin-bottom: 44px;
}
.section-head.row {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
}
.section-sub {
  margin: 10px 0 0;
}

/* ===== Шаги ===== */
.steps {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  border-top: 1px solid var(--border-strong);
}
.step {
  padding: 28px 28px 8px 0;
  border-right: 1px solid var(--border);
}
.step + .step {
  padding-left: 28px;
}
.step:last-child {
  border-right: none;
}
.step-num {
  display: block;
  font-size: 56px;
  font-weight: 500;
  line-height: 1;
  color: var(--accent);
  margin-bottom: 20px;
}
.step h3 {
  font-size: 20px;
  margin-bottom: 10px;
}
.step p {
  margin: 0;
  font-size: 15px;
  color: var(--muted);
}
.rules {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 24px;
  margin-top: 56px;
  padding-top: 32px;
  border-top: 1px solid var(--border);
}
.rule {
  display: flex;
  gap: 14px;
  font-size: 14px;
  color: var(--muted);
}
.rule strong {
  color: var(--text);
  font-weight: 600;
}
.rule .icon {
  color: var(--accent);
  margin-top: 2px;
}

/* ===== Плитки лотов ===== */
.tiles {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
}
.tile-skeleton {
  aspect-ratio: 1 / 1.08;
}

/* ===== Категории ===== */
.categories {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
}
.category {
  position: relative;
  aspect-ratio: 16 / 10;
  overflow: hidden;
  color: #fff;
}
.category img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.6s ease;
}
.category:hover img {
  transform: scale(1.05);
}
.category-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(0deg, rgb(0 0 0 / 0.65), rgb(0 0 0 / 0.05) 60%);
}
.category-name {
  position: absolute;
  left: 24px;
  bottom: 20px;
  font-size: 22px;
  font-weight: 500;
}
.category-arrow {
  position: absolute;
  right: 24px;
  bottom: 24px;
  transition: transform 0.2s;
}
.category:hover .category-arrow {
  transform: translateX(4px);
}

/* ===== Продавцам ===== */
.sell {
  display: grid;
  grid-template-columns: 1fr 1fr;
  background: var(--surface-2);
}
.sell-img {
  width: 100%;
  height: 100%;
  min-height: 360px;
  object-fit: cover;
}
.sell-body {
  padding: 64px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: flex-start;
  gap: 18px;
}
.sell-body p {
  margin: 0;
  color: var(--muted);
  max-width: 460px;
}

/* ===== FAQ ===== */
.faq-section {
  display: grid;
  grid-template-columns: 1fr 2fr;
  gap: 64px;
  align-items: start;
}
.faq {
  border-top: 1px solid var(--border-strong);
}
.faq-item {
  border-bottom: 1px solid var(--border);
}
.faq-item summary {
  list-style: none;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 24px;
  padding: 22px 0;
  font-size: 18px;
  font-weight: 500;
  color: var(--text);
  cursor: pointer;
}
.faq-item summary::-webkit-details-marker {
  display: none;
}
.faq-item summary:hover {
  color: var(--accent);
}
.faq-icon {
  color: var(--accent);
  transition: transform 0.25s;
}
.faq-item[open] .faq-icon {
  transform: rotate(45deg);
}
.faq-item p {
  margin: 0 0 24px;
  max-width: 680px;
  color: var(--muted);
}

/* ===== Адаптив ===== */
@media (max-width: 1000px) {
  .about,
  .faq-section {
    grid-template-columns: 1fr;
    gap: 40px;
  }
  .steps {
    grid-template-columns: 1fr 1fr;
  }
  .step:nth-child(2) {
    border-right: none;
  }
  .step:nth-child(3) {
    padding-left: 0;
  }
  .step:nth-child(n + 3) {
    border-top: 1px solid var(--border);
  }
  .tiles,
  .categories {
    grid-template-columns: 1fr 1fr;
  }
  .sell {
    grid-template-columns: 1fr;
  }
  .sell-img {
    min-height: 240px;
    max-height: 320px;
  }
}

@media (max-width: 640px) {
  .hero {
    min-height: 480px;
    height: calc(100svh - var(--header-h) - 64px);
  }
  .hero-shade {
    background: linear-gradient(0deg, rgb(0 0 0 / 0.78) 0%, rgb(0 0 0 / 0.35) 60%, rgb(0 0 0 / 0.2) 100%);
  }
  .hero-slide {
    align-items: flex-end;
  }
  .hero-content {
    padding-bottom: 88px;
  }
  .hero-text {
    font-size: 16px;
  }
  .hero-meta {
    font-size: 24px;
    gap: 12px;
  }
  .hero-line {
    width: 32px;
  }
  .hero-actions .btn {
    flex: 1;
  }
  .arrow {
    width: 44px;
  }
  .about {
    padding-top: 48px;
    padding-bottom: 48px;
  }
  .stats {
    gap: 12px;
  }
  .section {
    padding-top: 56px;
    padding-bottom: 56px;
  }
  .section-head {
    margin-bottom: 28px;
  }
  .steps {
    grid-template-columns: 1fr;
  }
  .step,
  .step + .step {
    padding: 22px 0;
    border-right: none;
    border-top: 1px solid var(--border);
  }
  .step:first-child {
    border-top: none;
  }
  .step-num {
    font-size: 40px;
    margin-bottom: 12px;
  }
  .rules {
    grid-template-columns: 1fr;
    margin-top: 24px;
  }
  .tiles,
  .categories {
    gap: 10px;
  }
  .category {
    aspect-ratio: 1;
  }
  .category-name {
    left: 12px;
    bottom: 12px;
    font-size: 15px;
  }
  .category-arrow {
    display: none;
  }
  .sell-body {
    padding: 28px 20px;
  }
  .sell-body .btn {
    width: 100%;
  }
  .faq-item summary {
    font-size: 16px;
  }
}
</style>
