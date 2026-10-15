import { VueQueryPlugin } from '@tanstack/vue-query'
import { createApp } from 'vue'
import App from './app/App.vue'
import { createAppRouter } from './app/router'
import { configureApi } from './infrastructure/http'

configureApi()
createApp(App).use(createAppRouter()).use(VueQueryPlugin).mount('#app')
