import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import './assets/main.css'
import { applyTheme } from './stores/ui'
import { onUnauthorized } from './api/client'
import { auth } from './stores/auth'

applyTheme()

onUnauthorized(() => {
  if (auth.user) {
    auth.user = null
    router.push({ name: 'login' })
  }
})

createApp(App).use(router).mount('#app')
