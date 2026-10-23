<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ApiError, api } from '../api/client'
import type { BidPlaced, Lot } from '../api/types'
import { useAuth } from '../stores/auth'
import { formatRub, rubToKop } from '../utils/format'
import { useCountdown } from '../composables/useCountdown'

const props = defineProps<{ lot: Lot }>()
const emit = defineEmits<{ placed: [BidPlaced] }>()

const auth = useAuth()
const { expired } = useCountdown(() => props.lot.endsAt)

const amountRub = ref(props.lot.minNextBid / 100)
const sending = ref(false)
const error = ref('')
const success = ref('')
const input = ref<HTMLInputElement>()

// Двухшаговая ставка, как на реальных аукционах: «Сделать ставку» → «Подтвердить N ₽».
const confirming = ref(false)
let confirmTimer: ReturnType<typeof setTimeout>
watch(amountRub, () => (confirming.value = false))

defineExpose({
  focus() {
    input.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    input.value?.focus({ preventScroll: true })
  },
})

// Когда кто-то перебил ставку, поднимаем значение в поле до нового минимума.
watch(
  () => props.lot.minNextBid,
  (min) => {
    if (rubToKop(amountRub.value || 0) < min) amountRub.value = min / 100
  },
)

const isOwner = computed(() => auth.user?.id === props.lot.sellerId)
const isLeader = computed(() => auth.user != null && auth.user.id === props.lot.leaderId)
const isOpen = computed(() => props.lot.status === 'active' && !expired.value)
const tooLow = computed(() => rubToKop(amountRub.value || 0) < props.lot.minNextBid)

const steps = [1, 2, 5, 10]
const stepAmount = (n: number) => props.lot.minNextBid + props.lot.minStep * (n - 1)

function setSteps(n: number) {
  amountRub.value = stepAmount(n) / 100
}

async function submit() {
  error.value = ''
  success.value = ''
  if (tooLow.value) {
    error.value = `Минимальная ставка — ${formatRub(props.lot.minNextBid)}`
    return
  }
  if (!confirming.value) {
    confirming.value = true
    clearTimeout(confirmTimer)
    confirmTimer = setTimeout(() => (confirming.value = false), 5000)
    return
  }
  confirming.value = false
  sending.value = true
  try {
    const res = await api.placeBid(props.lot.id, rubToKop(amountRub.value))
    success.value = `Ставка ${formatRub(res.currentPrice)} принята — вы лидируете!`
    emit('placed', res)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'bid_too_low' && typeof e.details?.minNextBid === 'number') {
      error.value = `Вас опередили. Минимальная ставка теперь ${formatRub(e.details.minNextBid)}`
      amountRub.value = e.details.minNextBid / 100
    } else {
      error.value = e instanceof Error ? e.message : 'Не удалось сделать ставку'
    }
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="bid-panel">
    <template v-if="!isOpen" />
    <div v-else-if="!auth.user" class="guest">
      <p class="muted">Чтобы делать ставки, войдите или зарегистрируйтесь — это займёт минуту.</p>
      <RouterLink :to="{ name: 'login', query: { redirect: `/lots/${lot.id}` } }" class="btn btn-primary">Войти и сделать ставку</RouterLink>
    </div>
    <div v-else-if="isOwner" class="alert">Это ваш лот — ставки на него делать нельзя</div>
    <form v-else @submit.prevent="submit">
      <div v-if="isLeader" class="alert alert-success">{{ success || 'Ваша ставка лидирует' }}</div>
      <label class="field">
        <span>Ваша ставка, ₽ · минимум {{ formatRub(lot.minNextBid) }}</span>
        <div class="row">
          <input
            ref="input"
            v-model.number="amountRub"
            class="input tabular amount-input"
            type="number"
            inputmode="decimal"
            :min="lot.minNextBid / 100"
            :step="lot.minStep / 100"
            :disabled="sending"
          />
          <button class="btn btn-primary submit" :class="{ confirm: confirming }" :disabled="sending || isLeader || tooLow">
            {{ sending ? 'Отправка…' : confirming ? `Подтвердить ${formatRub(rubToKop(amountRub))}` : 'Сделать ставку' }}
          </button>
        </div>
      </label>
      <div class="quick">
        <button
          v-for="n in steps"
          :key="n"
          type="button"
          class="btn btn-sm tabular"
          :class="{ selected: rubToKop(amountRub || 0) === stepAmount(n) }"
          @click="setSteps(n)"
        >
          {{ formatRub(stepAmount(n)) }}
        </button>
      </div>
      <p v-if="confirming" class="hint muted">Нажмите ещё раз, чтобы подтвердить. Ставку нельзя отменить.</p>
      <div v-if="error" class="alert alert-error">{{ error }}</div>
    </form>
  </div>
</template>

<style scoped>
form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.row {
  display: flex;
  gap: 8px;
}
.row .input {
  flex: 1;
  min-width: 0;
}
.amount-input {
  font-family: var(--serif);
  font-size: 24px;
  font-weight: 600;
  height: 52px;
}
.submit {
  min-width: 190px;
  height: 52px;
}
.guest {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.guest p {
  margin: 0;
  font-size: 14px;
}
.submit.confirm {
  background: var(--success);
  border-color: var(--success);
}
.quick {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 6px;
}
.quick .btn {
  font-family: var(--serif);
  font-size: 16px;
  letter-spacing: 0;
  text-transform: none;
  font-weight: 600;
}
.quick .selected {
  border-color: var(--accent);
  color: var(--accent);
}
.hint {
  margin: 0;
  font-size: 13px;
}
@media (max-width: 400px) {
  .row {
    flex-direction: column;
  }
  .row .input {
    flex: none;
  }
  .quick {
    grid-template-columns: repeat(2, 1fr);
  }
}
.link {
  color: var(--accent);
  font-weight: 600;
}
</style>
