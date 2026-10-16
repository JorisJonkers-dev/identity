import { createRouter, createWebHistory, type Router, type RouterHistory } from 'vue-router'

/** The app's routes. Each feature page loads on demand, so the first paint carries only the shell. */
export function createAppRouter(history: RouterHistory = createWebHistory()): Router {
  return createRouter({
    history,
    routes: [
      { path: '/', name: 'home', component: () => import('../features/home/components/HomePage.vue') },
      { path: '/signup', name: 'signup', component: () => import('../features/account/components/SignUpPage.vue') },
      { path: '/confirm', name: 'confirm', component: () => import('../features/account/components/ConfirmPage.vue') },
      { path: '/signin', name: 'signin', component: () => import('../features/account/components/SignInPage.vue') },
      { path: '/profile', name: 'profile', component: () => import('../features/account/components/ProfilePage.vue') },
      { path: '/security', name: 'security', component: () => import('../features/security/components/SecurityPage.vue') },
      { path: '/access', name: 'access', component: () => import('../features/access/components/AccessPage.vue') },
      { path: '/no-access', name: 'no-access', component: () => import('../features/access/components/NoAccessPage.vue') },
      // The admin pages fill the window, with their own sidebar.
      { path: '/admin', name: 'dashboard', meta: { wide: true }, component: () => import('../features/admin/components/DashboardPage.vue') },
      { path: '/admin/audit', name: 'audit', meta: { wide: true }, component: () => import('../features/admin/components/AuditPage.vue') },
      { path: '/:rest(.*)*', redirect: '/' },
    ],
  })
}
