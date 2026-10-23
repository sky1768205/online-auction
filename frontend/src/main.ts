import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { socket } from './realtime/socket'
import './style.css'

createApp(App).use(createPinia()).use(router).mount('#app')
socket.connect()
