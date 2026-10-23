<script setup lang="ts">
import type { LotStatus } from '../api/types'
import { useCountdown } from '../composables/useCountdown'

const props = defineProps<{ endsAt: string; status: LotStatus; large?: boolean }>()
const { text, expired, urgent } = useCountdown(() => props.endsAt)
</script>

<template>
  <span class="countdown tabular" :class="{ large, urgent: urgent && status === 'active' }">
    <template v-if="status === 'cancelled'">Снят с торгов</template>
    <template v-else-if="status !== 'active'">Торги завершены</template>
    <!-- Таймер истёк, но сервер ещё не прислал lot:closed (закрытие раз в секунду) -->
    <template v-else-if="expired">Подводим итоги…</template>
    <template v-else>{{ text }}</template>
  </span>
</template>

<style scoped>
.countdown {
  font-weight: 600;
}
.large {
  font-family: var(--serif);
  font-size: clamp(36px, 3.4vw, 46px);
  line-height: 1;
  color: var(--text);
}
.countdown.urgent {
  color: var(--danger);
  animation: pulse 1s ease-in-out infinite;
}
@keyframes pulse {
  50% {
    opacity: 0.55;
  }
}
</style>
