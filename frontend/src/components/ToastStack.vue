<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useToasts, type Toast } from '../stores/toasts'
import Icon from './Icon.vue'

const toasts = useToasts()
const router = useRouter()

const icons = { info: 'bell', success: 'trophy', warning: 'bolt' }

function open(t: Toast) {
  if (t.to) router.push(t.to)
  toasts.dismiss(t.id)
}
</script>

<template>
  <div class="stack" aria-live="polite">
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts.items"
        :key="t.id"
        class="toast"
        :class="[t.kind, { clickable: t.to }]"
        role="status"
        @click="open(t)"
      >
        <span class="icon"><Icon :name="icons[t.kind]" /></span>
        <div class="body">
          <div class="title">{{ t.title }}</div>
          <div v-if="t.text" class="text muted">{{ t.text }}</div>
        </div>
        <button class="close" aria-label="Закрыть" @click.stop="toasts.dismiss(t.id)"><Icon name="close" :size="16" /></button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.stack {
  position: fixed;
  top: calc(var(--header-h) + 16px);
  right: 16px;
  z-index: 100;
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: min(360px, calc(100vw - 32px));
  pointer-events: none;
}
.toast {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 12px 14px;
  pointer-events: auto;
  background: var(--surface);
  border: 1px solid var(--border);
  border-left: 2px solid var(--accent);
  box-shadow: var(--shadow-float);
}
.toast.success {
  border-left-color: var(--success);
}
.toast.warning {
  border-left-color: var(--danger);
}
.clickable {
  cursor: pointer;
}
.icon {
  color: var(--accent);
  padding-top: 1px;
}
.success .icon {
  color: var(--success);
}
.warning .icon {
  color: var(--danger);
}
.body {
  flex: 1;
  min-width: 0;
}
.title {
  font-weight: 600;
  color: var(--text);
}
.text {
  font-size: 13px;
}
.close {
  border: none;
  background: none;
  color: var(--muted);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
  padding: 0 2px;
}
.toast-enter-active,
.toast-leave-active {
  transition: all 0.25s ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(24px);
}
.toast-leave-to {
  opacity: 0;
  transform: scale(0.95);
}
@media (max-width: 640px) {
  .stack {
    top: calc(var(--header-h) + 8px);
    left: 16px;
    right: 16px;
    width: auto;
  }
  .toast-enter-from {
    transform: translateY(-16px);
  }
}
</style>
