<script setup lang="ts">
import { computed } from 'vue'
import type { Lot } from '../api/types'
import { formatRub, pluralBids } from '../utils/format'
import { useCountdown } from '../composables/useCountdown'
import Countdown from './Countdown.vue'

// Плитка в музейном стиле: фотография на всю карточку, текст поверх затемнения.
const props = defineProps<{ lot: Lot; flash?: boolean }>()
const { remainingMs } = useCountdown(() => props.lot.endsAt)

const status = computed(() => {
  if (props.lot.status === 'cancelled') return 'Снят с торгов'
  if (props.lot.status !== 'active') return props.lot.winnerId ? 'Продан' : 'Не продан'
  if (remainingMs.value > 0 && remainingMs.value < 10 * 60_000) return 'Скоро закончится'
  return props.lot.bidCount ? 'Идут торги' : 'Без ставок'
})
const urgent = computed(() => props.lot.status === 'active' && remainingMs.value > 0 && remainingMs.value < 10 * 60_000)
const price = computed(() =>
  props.lot.status === 'active' ? (props.lot.currentPrice ?? props.lot.startPrice) : props.lot.finalPrice,
)
const priceLabel = computed(() =>
  props.lot.status !== 'active' ? 'Итог' : props.lot.currentPrice ? 'Текущая ставка' : 'Стартовая цена',
)
</script>

<template>
  <RouterLink :to="`/lots/${lot.id}`" class="tile" :class="{ finished: lot.status !== 'active' }">
    <img v-if="lot.coverUrl" class="tile-img" :src="lot.coverUrl" :alt="lot.title" loading="lazy" />
    <div v-else class="tile-img no-image" />
    <div class="tile-shade" />

    <div class="tile-top">
      <span class="caps status" :class="{ urgent }">{{ status }}</span>
      <span v-if="lot.status === 'active'" class="timer" :class="{ urgent }">
        <Countdown :ends-at="lot.endsAt" :status="lot.status" />
      </span>
    </div>

    <div class="tile-bottom">
      <div class="caps category">{{ lot.categoryName }}</div>
      <h3 class="title">{{ lot.title }}</h3>
      <div :key="lot.bidCount" class="price-row" :class="{ flash }">
        <span class="caps label">{{ priceLabel }}</span>
        <span class="price serif tabular">{{ price != null ? formatRub(price) : '—' }}</span>
      </div>
      <div class="sub">
        {{ pluralBids(lot.bidCount) }}<template v-if="lot.status !== 'active' && lot.winnerName"> · {{ lot.winnerName }}</template>
      </div>
    </div>
  </RouterLink>
</template>

<style scoped>
.tile {
  position: relative;
  display: block;
  aspect-ratio: 1 / 1.08;
  overflow: hidden;
  background: #1a1815;
  color: #fff;
}
.tile-img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.7s ease;
}
.tile:hover .tile-img {
  transform: scale(1.05);
}
.finished .tile-img {
  filter: grayscale(0.7);
}
.no-image {
  background: linear-gradient(135deg, #2a2621, #15130f);
}
.tile-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(0deg, rgb(0 0 0 / 0.82) 0%, rgb(0 0 0 / 0.25) 48%, rgb(0 0 0 / 0.35) 100%);
}
.tile-top {
  position: absolute;
  top: 20px;
  left: 20px;
  right: 20px;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}
.status {
  font-size: 11px;
  max-width: 50%;
}
.status.urgent {
  color: #ffb4a5;
}
.timer {
  padding: 6px 12px;
  border: 1px solid rgb(255 255 255 / 0.6);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.timer.urgent {
  border-color: #ffb4a5;
  color: #ffb4a5;
}
.timer :deep(.countdown) {
  font-weight: 600;
  animation: none;
  color: inherit;
}
.tile-bottom {
  position: absolute;
  left: 20px;
  right: 20px;
  bottom: 20px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.category {
  font-size: 10.5px;
  color: rgb(255 255 255 / 0.7);
}
.title {
  color: #fff;
  font-size: 21px;
  font-weight: 500;
  line-height: 1.2;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.price-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-top: 4px;
  padding-top: 12px;
  border-top: 1px solid rgb(255 255 255 / 0.25);
}
.price-row.flash {
  animation: tile-flash 1.2s ease-out;
}
@keyframes tile-flash {
  from {
    background: rgb(180 150 110 / 0.55);
  }
  to {
    background: transparent;
  }
}
.label {
  font-size: 10px;
  color: rgb(255 255 255 / 0.65);
}
.price {
  margin-left: auto;
  font-size: 28px;
  font-weight: 600;
  line-height: 1;
}
.sub {
  font-size: 12px;
  color: rgb(255 255 255 / 0.6);
}

@media (max-width: 640px) {
  .tile {
    aspect-ratio: 3 / 4.3;
  }
  .tile-top {
    top: 10px;
    left: 10px;
    right: 10px;
    flex-direction: column;
    gap: 6px;
  }
  .status {
    font-size: 9px;
    max-width: none;
  }
  .timer {
    padding: 3px 7px;
    font-size: 11px;
  }
  .tile-bottom {
    left: 10px;
    right: 10px;
    bottom: 10px;
    gap: 4px;
  }
  .category {
    display: none;
  }
  .title {
    font-size: 14px;
  }
  .price-row {
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    padding-top: 8px;
  }
  .price {
    margin-left: 0;
    font-size: 21px;
  }
  .sub {
    font-size: 11px;
  }
}
</style>
