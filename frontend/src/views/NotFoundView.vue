<template>
  <div class="not-found">
    <h1>Страница не найдена</h1>
    <RouterLink to="/catalog" class="btn">В каталог</RouterLink>
  </div>
</template>

<style scoped>
.not-found {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 80px 0;
}
</style>
