<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { LotImage } from '../api/types'
import Icon from './Icon.vue'

const props = defineProps<{ images: LotImage[]; alt: string }>()
const index = ref(0)
const fullscreen = ref(false)
const current = computed(() => props.images[index.value])

watch(() => props.images, () => (index.value = 0))

function go(delta: number) {
  const n = props.images.length
  if (n > 1) index.value = (index.value + delta + n) % n
}

// Свайп пальцем (и мышью) — листаем, если сдвиг по горизонтали больше 40px
// и он заметнее вертикального (чтобы не мешать прокрутке страницы).
let startX = 0
let startY = 0
let swiped = false
function onPointerDown(e: PointerEvent) {
  startX = e.clientX
  startY = e.clientY
  swiped = false
}
function onPointerUp(e: PointerEvent) {
  const dx = e.clientX - startX
  const dy = e.clientY - startY
  if (Math.abs(dx) > 40 && Math.abs(dx) > Math.abs(dy)) {
    go(dx < 0 ? 1 : -1)
    swiped = true
  }
}
function openFullscreen() {
  if (!swiped && current.value) fullscreen.value = true
}

function onKey(e: KeyboardEvent) {
  if (!fullscreen.value) return
  if (e.key === 'Escape') fullscreen.value = false
  if (e.key === 'ArrowLeft') go(-1)
  if (e.key === 'ArrowRight') go(1)
}
watch(fullscreen, (open) => {
  document.body.style.overflow = open ? 'hidden' : ''
  if (open) window.addEventListener('keydown', onKey)
  else window.removeEventListener('keydown', onKey)
})
onUnmounted(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="gallery" tabindex="0" @keydown.left="go(-1)" @keydown.right="go(1)">
    <div class="main" @pointerdown="onPointerDown" @pointerup="onPointerUp" @click="openFullscreen">
      <img v-if="current" :src="current.url" :alt="`${alt} — фото ${index + 1}`" draggable="false" />
      <div v-else class="empty muted">Нет фотографий</div>
      <template v-if="images.length > 1">
        <button class="nav prev" aria-label="Предыдущее фото" @click.stop="go(-1)">‹</button>
        <button class="nav next" aria-label="Следующее фото" @click.stop="go(1)">›</button>
        <div class="dots">
          <span v-for="(_, i) in images" :key="i" class="dot" :class="{ active: i === index }" />
        </div>
      </template>
      <span v-if="current" class="zoom badge"><Icon name="expand" :size="14" /> На весь экран</span>
    </div>
    <div v-if="images.length > 1" class="thumbs">
      <button
        v-for="(img, i) in images"
        :key="img.id"
        class="thumb"
        :class="{ active: i === index }"
        :aria-label="`Фото ${i + 1}`"
        @click="index = i"
      >
        <img :src="img.url" alt="" loading="lazy" />
      </button>
    </div>

    <Teleport to="body">
      <Transition name="fade">
        <div v-if="fullscreen && current" class="lightbox" @click.self="fullscreen = false" @pointerdown="onPointerDown" @pointerup="onPointerUp">
          <img :src="current.url" :alt="alt" draggable="false" />
          <button class="lb-close" aria-label="Закрыть" @click="fullscreen = false">×</button>
          <template v-if="images.length > 1">
            <button class="nav prev" aria-label="Предыдущее фото" @click.stop="go(-1)">‹</button>
            <button class="nav next" aria-label="Следующее фото" @click.stop="go(1)">›</button>
            <span class="lb-counter">{{ index + 1 }} / {{ images.length }}</span>
          </template>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.gallery {
  display: flex;
  flex-direction: column;
  gap: 10px;
  outline: none;
}
.main {
  position: relative;
  aspect-ratio: 4 / 3;
  overflow: hidden;
  background: var(--surface-2);
  cursor: zoom-in;
  touch-action: pan-y; /* вертикальная прокрутка — браузеру, горизонтальный свайп — нам */
  user-select: none;
}
.main img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.empty {
  display: grid;
  place-items: center;
  height: 100%;
}
.nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 44px;
  height: 44px;
  border-radius: 0;
  border: none;
  background: rgb(20 20 20 / 0.55);
  color: #fff;
  font-size: 26px;
  line-height: 1;
  cursor: pointer;
}
.prev {
  left: 12px;
}
.next {
  right: 12px;
}
.dots {
  position: absolute;
  bottom: 12px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 6px;
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: rgb(255 255 255 / 0.5);
  transition: all 0.2s;
}
.dot.active {
  background: #fff;
  width: 18px;
  border-radius: 4px;
}
.zoom {
  position: absolute;
  top: 12px;
  right: 12px;
  background: color-mix(in srgb, var(--surface) 85%, transparent);
  color: var(--text);
  opacity: 0;
  transition: opacity 0.2s;
}
.main:hover .zoom {
  opacity: 1;
}
.thumbs {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  scrollbar-width: none;
}
.thumb {
  flex: 0 0 92px;
  height: 69px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 0;
  overflow: hidden;
  cursor: pointer;
  background: var(--surface-2);
  opacity: 0.65;
}
.thumb.active {
  border-color: var(--accent);
  outline: 1px solid var(--accent);
  opacity: 1;
}
.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.lightbox {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: rgb(0 0 0 / 0.92);
  display: grid;
  place-items: center;
  touch-action: pan-y;
}
.lightbox img {
  max-width: 96vw;
  max-height: 90vh;
  object-fit: contain;
  user-select: none;
}
.lightbox .nav {
  background: rgb(255 255 255 / 0.15);
  color: #fff;
  width: 48px;
  height: 48px;
}
.lb-close {
  position: absolute;
  top: calc(12px + env(safe-area-inset-top));
  right: 16px;
  width: 44px;
  height: 44px;
  border: none;
  border-radius: 50%;
  background: rgb(255 255 255 / 0.15);
  color: #fff;
  font-size: 28px;
  cursor: pointer;
}
.lb-counter {
  position: absolute;
  bottom: calc(20px + env(safe-area-inset-bottom));
  color: #fff;
  font-size: 14px;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

@media (max-width: 640px) {
  /* На телефоне фото во всю ширину экрана, без миниатюр — листаем свайпом */
  .main {
    margin: 0 -16px;
    border-radius: 0;
    border-left: none;
    border-right: none;
  }
  .main .nav,
  .thumbs,
  .zoom {
    display: none;
  }
}
</style>
