<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import type { Category } from '../api/types'
import { rubToKop } from '../utils/format'
import Icon from '../components/Icon.vue'

const router = useRouter()
const categories = ref<Category[]>([])

const title = ref('')
const description = ref('')
const categoryId = ref<number>()
const startPrice = ref<number>(1000)
const minStep = ref<number>(100)
const duration = ref(60)
const images = ref<string[]>([])
const uploading = ref(false)
const sending = ref(false)
const error = ref('')

const durations = [
  { label: '5 минут', value: 5 },
  { label: '15 минут', value: 15 },
  { label: '1 час', value: 60 },
  { label: '6 часов', value: 360 },
  { label: '1 день', value: 1440 },
  { label: '3 дня', value: 4320 },
  { label: '7 дней', value: 10080 },
]

onMounted(async () => {
  categories.value = await api.categories()
  categoryId.value = categories.value[0]?.id
})

async function onFiles(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files ?? []).slice(0, 10 - images.value.length)
  uploading.value = true
  error.value = ''
  try {
    for (const file of files) images.value.push((await api.upload(file)).url)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Не удалось загрузить файл'
  } finally {
    uploading.value = false
    input.value = ''
  }
}

async function submit() {
  error.value = ''
  sending.value = true
  try {
    const lot = await api.createLot({
      title: title.value,
      description: description.value,
      categoryId: categoryId.value!,
      startPrice: rubToKop(startPrice.value),
      minStep: rubToKop(minStep.value),
      durationMinutes: duration.value,
      images: images.value,
    })
    router.push(`/lots/${lot.id}`)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Не удалось создать лот'
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <form class="create" @submit.prevent="submit">
    <div class="caps muted kicker">Продавцам</div>
    <h1 class="page-title">Новый лот</h1>
    <p class="muted lead">Заполните описание, задайте стартовую цену и шаг торгов — лот сразу появится в каталоге.</p>

    <h2 class="group-title">Описание</h2>

    <label class="field">
      <span>Название</span>
      <input v-model="title" class="input" required minlength="3" maxlength="200" />
    </label>

    <label class="field">
      <span>Категория</span>
      <select v-model="categoryId" class="select" required>
        <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
    </label>

    <label class="field">
      <span>Описание</span>
      <textarea v-model="description" class="textarea" maxlength="10000" />
    </label>

    <h2 class="group-title">Условия торгов</h2>
    <div class="row">
      <label class="field">
        <span>Стартовая цена, ₽</span>
        <input v-model.number="startPrice" class="input" type="number" min="1" step="0.01" required />
      </label>
      <label class="field">
        <span>Шаг торгов, ₽</span>
        <input v-model.number="minStep" class="input" type="number" min="1" step="0.01" required />
      </label>
      <label class="field">
        <span>Длительность</span>
        <select v-model="duration" class="select">
          <option v-for="d in durations" :key="d.value" :value="d.value">{{ d.label }}</option>
        </select>
      </label>
    </div>

    <h2 class="group-title">Фотографии</h2>
    <div class="field">
      <span>До 10 файлов · первая фотография станет обложкой</span>
      <div class="images">
        <div v-for="(url, i) in images" :key="url" class="image">
          <img :src="url" alt="" />
          <button type="button" class="remove" aria-label="Удалить" @click="images.splice(i, 1)"><Icon name="close" :size="14" /></button>
        </div>
        <label v-if="images.length < 10" class="add" :class="{ busy: uploading }">
          <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" multiple hidden :disabled="uploading" @change="onFiles" />
          <Icon name="plus" /> {{ uploading ? 'Загрузка…' : 'Добавить' }}
        </label>
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <button class="btn btn-primary submit" :disabled="sending || uploading">Выставить на торги</button>
  </form>
</template>

<style scoped>
.create {
  max-width: 760px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 22px;
}
.kicker {
  margin-bottom: -10px;
}
.page-title {
  font-size: clamp(34px, 4.4vw, 54px);
  letter-spacing: -0.02em;
}
.lead {
  margin: -6px 0 0;
}
.group-title {
  font-size: 22px;
  margin-top: 20px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-strong);
}
.row {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.images {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.image,
.add {
  position: relative;
  width: 140px;
  height: 105px;
  overflow: hidden;
}
.image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.image:first-child::after {
  content: 'Обложка';
  position: absolute;
  left: 0;
  bottom: 0;
  padding: 3px 8px;
  background: var(--accent);
  color: var(--on-accent);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}
.remove {
  position: absolute;
  top: 6px;
  right: 6px;
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: none;
  background: rgb(0 0 0 / 0.65);
  color: #fff;
  cursor: pointer;
}
.add {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px dashed var(--border-strong);
  color: var(--muted);
  cursor: pointer;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  transition: border-color 0.2s, color 0.2s;
}
.add:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.add.busy {
  cursor: progress;
}
.submit {
  margin-top: 16px;
  align-self: flex-start;
  min-width: 260px;
}
@media (max-width: 640px) {
  .row {
    grid-template-columns: 1fr;
  }
  .image,
  .add {
    width: calc(50% - 6px);
    height: 120px;
  }
  .submit {
    width: 100%;
  }
}
</style>
