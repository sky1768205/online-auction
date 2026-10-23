<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { applyBid, applyClosed, type BidPlaced, type Category, type Lot, type LotCancelled, type LotClosed } from '../api/types'
import { socket } from '../realtime/socket'
import LotCard from '../components/LotCard.vue'
import Icon from '../components/Icon.vue'
import { plural } from '../utils/format'

const route = useRoute()
const router = useRouter()

const categories = ref<Category[]>([])
const lots = ref<Lot[]>([])
const total = ref(0)
const limit = 24
const loading = ref(false)
const firstLoad = ref(true)
const error = ref('')
const newLots = ref(0)
const flashed = reactive(new Set<number>())

// Фильтры живут в URL — ссылкой на отфильтрованный каталог можно поделиться.
const filters = computed(() => ({
  category: (route.query.category as string) || '',
  status: route.query.status === 'finished' ? 'finished' : 'active',
  q: (route.query.q as string) || '',
  sort: (route.query.sort as string) || 'ending',
  page: Math.max(1, Number(route.query.page) || 1),
}))
const pages = computed(() => Math.max(1, Math.ceil(total.value / limit)))
const search = ref(filters.value.q)

function setFilter(patch: Record<string, string | number | undefined>) {
  const query = { ...route.query, ...patch }
  if (!('page' in patch)) delete query.page
  for (const k of Object.keys(query)) if (query[k] === '' || query[k] === undefined) delete query[k]
  router.replace({ query: query as Record<string, string> })
}

let searchTimer: ReturnType<typeof setTimeout>
watch(search, (q) => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => setFilter({ q }), 300)
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.lots({ ...filters.value, limit })
    lots.value = res.items
    total.value = res.total
    newLots.value = 0
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Не удалось загрузить лоты'
  } finally {
    loading.value = false
    firstLoad.value = false
  }
}

watch(filters, load, { immediate: true })

function onEvent(type: string, data: unknown) {
  if (type === 'bid:new') {
    const e = data as BidPlaced
    const lot = lots.value.find((l) => l.id === e.lotId)
    if (lot) {
      applyBid(lot, e)
      flashed.add(lot.id)
      setTimeout(() => flashed.delete(lot.id), 1200)
    }
  } else if (type === 'lot:closed') {
    const e = data as LotClosed
    const lot = lots.value.find((l) => l.id === e.lotId)
    if (lot) applyClosed(lot, e)
  } else if (type === 'lot:cancelled') {
    // Снятый лот сразу убираем из публичного каталога
    lots.value = lots.value.filter((l) => l.id !== (data as LotCancelled).lotId)
  } else if (type === 'lot:created' && filters.value.status === 'active') {
    newLots.value++
  }
}

let unsubscribe = () => {}
let offReconnect = () => {}
onMounted(async () => {
  unsubscribe = socket.subscribe('catalog', onEvent)
  offReconnect = socket.onReconnect(load)
  categories.value = await api.categories().catch(() => [])
})
onUnmounted(() => {
  unsubscribe()
  offReconnect()
})
</script>

<template>
  <div class="catalog">
    <header class="head">
      <div>
        <div class="caps muted kicker">Аукционы</div>
        <h1 class="page-title">Каталог лотов</h1>
      </div>
      <nav class="tabs" role="tablist">
        <button class="tab caps" :class="{ active: filters.status === 'active' }" @click="setFilter({ status: '' })">Идут торги</button>
        <button class="tab caps" :class="{ active: filters.status === 'finished' }" @click="setFilter({ status: 'finished' })">Завершённые</button>
      </nav>
    </header>

    <div class="toolbar">
      <label class="search">
        <Icon name="search" :size="18" />
        <input v-model="search" class="input input-line" type="search" placeholder="Поиск по названию" />
      </label>
      <label class="sort">
        <span class="caps muted">Сортировка</span>
        <select class="select-plain" :value="filters.sort" @change="setFilter({ sort: ($event.target as HTMLSelectElement).value })">
          <option value="ending">{{ filters.status === 'active' ? 'Скоро завершатся' : 'Недавно завершённые' }}</option>
          <option value="newest">Новые</option>
          <option value="popular">Больше ставок</option>
          <option value="price_asc">Сначала дешевле</option>
          <option value="price_desc">Сначала дороже</option>
        </select>
      </label>
    </div>

    <div class="filters">
      <span class="filters-label">Категория:</span>
      <div class="chips">
        <button class="chip" :class="{ active: !filters.category }" @click="setFilter({ category: '' })">#Все</button>
        <button
          v-for="c in categories"
          :key="c.id"
          class="chip"
          :class="{ active: filters.category === c.slug }"
          @click="setFilter({ category: c.slug })"
        >
          #{{ c.name }}
        </button>
      </div>
    </div>

    <div class="summary">
      <span v-if="!firstLoad" class="caps muted">{{ plural(total, 'лот', 'лота', 'лотов') }}</span>
      <button v-if="newLots" class="link-arrow caps new-lots" @click="load">
        Новых лотов: {{ newLots }} — показать <Icon name="arrowRight" :size="16" />
      </button>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-else-if="!loading && lots.length === 0" class="empty">
      <Icon name="search" :size="40" />
      <p>Ничего не найдено</p>
      <span class="muted">Попробуйте изменить запрос или категорию</span>
      <button v-if="filters.q || filters.category" class="btn btn-sm" @click="search = ''; setFilter({ q: '', category: '' })">Сбросить фильтры</button>
    </div>

    <div class="grid" :class="{ loading: loading && !firstLoad }">
      <template v-if="firstLoad">
        <div v-for="i in 6" :key="i" class="skeleton sk-tile" />
      </template>
      <LotCard v-for="lot in lots" v-else :key="lot.id" :lot="lot" :flash="flashed.has(lot.id)" />
    </div>

    <nav v-if="pages > 1" class="pager">
      <button class="icon-btn" aria-label="Назад" :disabled="filters.page <= 1" @click="setFilter({ page: filters.page - 1 })"><Icon name="arrowLeft" /></button>
      <span class="tabular">{{ filters.page }} / {{ pages }}</span>
      <button class="icon-btn" aria-label="Вперёд" :disabled="filters.page >= pages" @click="setFilter({ page: filters.page + 1 })"><Icon name="arrowRight" /></button>
    </nav>
  </div>
</template>

<style scoped>
.catalog {
  display: flex;
  flex-direction: column;
}
.head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
  padding-bottom: 28px;
  border-bottom: 1px solid var(--border);
}
.kicker {
  margin-bottom: 12px;
}
.page-title {
  font-size: clamp(34px, 4.4vw, 54px);
  letter-spacing: -0.02em;
}
.tabs {
  display: flex;
  gap: 28px;
}
.tab {
  border: none;
  background: none;
  padding: 8px 0;
  color: var(--muted);
  border-bottom: 1px solid transparent;
  cursor: pointer;
  font-family: inherit;
}
.tab:hover {
  color: var(--text);
}
.tab.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 32px;
  margin-top: 28px;
}
.search {
  flex: 1;
  max-width: 480px;
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
.sort {
  display: flex;
  align-items: center;
  gap: 12px;
}
.sort .caps {
  font-size: 11px;
}
.select-plain {
  border: none;
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  padding: 6px 0;
}
.select-plain:focus-visible {
  outline: 2px solid var(--accent);
}
.filters {
  display: flex;
  align-items: baseline;
  gap: 20px;
  margin-top: 28px;
}
.filters-label {
  font-size: 17px;
  font-weight: 500;
  color: var(--text);
  flex-shrink: 0;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 22px;
}
.chip {
  border: none;
  background: none;
  padding: 2px 0;
  font: inherit;
  font-size: 17px;
  color: var(--muted);
  border-bottom: 1px solid transparent;
  cursor: pointer;
  transition: color 0.2s, border-color 0.2s;
}
.chip:hover {
  color: var(--text);
}
.chip.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
}
.summary {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin: 36px 0 16px;
  min-height: 20px;
}
.new-lots {
  border: none;
  background: none;
  cursor: pointer;
  font-family: inherit;
}
.grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
  transition: opacity 0.2s;
}
.grid.loading {
  opacity: 0.5;
}
.sk-tile {
  aspect-ratio: 1 / 1.08;
}
.empty {
  padding: 72px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  text-align: center;
  color: var(--muted);
}
.empty p {
  margin: 8px 0 0;
  font-size: 22px;
  color: var(--text);
}
.empty .btn {
  margin-top: 12px;
}
.pager {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 20px;
  margin-top: 48px;
  font-size: 16px;
}
.pager .icon-btn:disabled {
  opacity: 0.3;
  cursor: default;
}

@media (max-width: 1000px) {
  .grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .head {
    padding-bottom: 16px;
  }
  .tabs {
    gap: 20px;
  }
  .toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
    margin-top: 16px;
  }
  .search {
    max-width: none;
  }
  .filters {
    margin-top: 16px;
  }
  .filters-label {
    display: none;
  }
  /* Категории — горизонтальная лента со свайпом */
  .chips {
    flex-wrap: nowrap;
    overflow-x: auto;
    margin: 0 -16px;
    padding: 0 16px 6px;
    scrollbar-width: none;
  }
  .chips::-webkit-scrollbar {
    display: none;
  }
  .chip {
    flex-shrink: 0;
    font-size: 15px;
  }
  .summary {
    margin: 20px 0 12px;
  }
  .grid {
    gap: 10px;
  }
}
</style>
