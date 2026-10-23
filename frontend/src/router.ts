import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from './stores/auth'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: () => import('./views/HomeView.vue'), meta: { fullWidth: true } },
    { path: '/catalog', name: 'catalog', component: () => import('./views/CatalogView.vue') },
    { path: '/lots/new', name: 'create-lot', component: () => import('./views/CreateLotView.vue'), meta: { auth: true } },
    { path: '/lots/:id(\\d+)', name: 'lot', component: () => import('./views/LotView.vue'), props: (r) => ({ id: Number(r.params.id) }) },
    { path: '/admin', name: 'admin', component: () => import('./views/AdminView.vue'), meta: { auth: true, admin: true } },
    { path: '/me', name: 'me', component: () => import('./views/MyView.vue'), meta: { auth: true } },
    { path: '/login', name: 'login', component: () => import('./views/AuthView.vue'), props: { mode: 'login' } },
    { path: '/register', name: 'register', component: () => import('./views/AuthView.vue'), props: { mode: 'register' } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('./views/NotFoundView.vue') },
  ],
  scrollBehavior(to, from, saved) {
    if (saved) return saved
    // Якоря на главной (/#how, /#faq) — с отступом под липкую шапку.
    if (to.hash) return { el: to.hash, top: 80, behavior: 'smooth' }
    return to.path !== from.path ? { top: 0 } : undefined
  },
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  if (!auth.loaded) await auth.load()
  if (to.meta.auth && !auth.user) return { name: 'login', query: { redirect: to.fullPath } }
  if (to.meta.admin && auth.user?.role !== 'admin') return { name: 'home' }
})
