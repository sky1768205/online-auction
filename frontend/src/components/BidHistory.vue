<script setup lang="ts">
import type { Bid } from '../api/types'
import { useAuth } from '../stores/auth'
import { formatDateTime, formatRub } from '../utils/format'

withDefaults(defineProps<{ bids: Bid[]; total: number; open?: boolean }>(), { open: true })
const auth = useAuth()
</script>

<template>
  <section class="history">
    <h2 class="block-title">История ставок <span class="count serif">{{ total }}</span></h2>
    <p v-if="bids.length === 0" class="muted">{{ open ? 'Ставок пока нет — станьте первым!' : 'Ставок не было.' }}</p>
    <TransitionGroup v-else tag="ol" name="bid" class="list">
      <li v-for="(bid, i) in bids" :key="bid.id" :class="{ top: i === 0, mine: bid.userId === auth.user?.id }">
        <span class="who">
          {{ bid.userName }}
          <span v-if="bid.userId === auth.user?.id" class="badge badge-accent">Вы</span>
        </span>
        <span class="when muted tabular">{{ formatDateTime(bid.createdAt) }}</span>
        <span class="amount serif tabular">{{ formatRub(bid.amount) }}</span>
      </li>
    </TransitionGroup>
  </section>
</template>

<style scoped>
.block-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
  font-size: 26px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--border-strong);
}
.count {
  font-size: 26px;
  color: var(--accent);
}
.history > p {
  margin: 20px 0 0;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 420px;
  overflow-y: auto;
}
li {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 16px;
  align-items: center;
  padding: 14px 0;
  border-bottom: 1px solid var(--border);
}
li.top .amount {
  color: var(--accent);
}
.who {
  font-weight: 500;
  display: flex;
  gap: 6px;
  align-items: center;
}
.when {
  font-size: 13px;
}
.amount {
  font-size: 20px;
  font-weight: 600;
  color: var(--text);
  min-width: 110px;
  text-align: right;
}
.bid-enter-active {
  transition: all 0.4s ease;
}
.bid-enter-from {
  opacity: 0;
  transform: translateY(-12px);
  background: var(--accent-soft);
}
@media (max-width: 640px) {
  .block-title,
  .count {
    font-size: 22px;
  }
  .when {
    display: none;
  }
}
</style>
