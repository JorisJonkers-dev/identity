import { createRouter, createWebHistory, type Router, type RouterHistory } from 'vue-router'

/** The app's routes. Each feature page loads on demand, so the first paint carries only the shell. */
export function createAppRouter(history: RouterHistory = createWebHistory()): Router {
  return createRouter({
    history,
    routes: [
      { path: '/', name: 'notes', component: () => import('../features/notes/components/NotesPage.vue') },
      { path: '/:rest(.*)*', redirect: '/' },
    ],
  })
}
