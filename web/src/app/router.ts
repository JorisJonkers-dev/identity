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
      { path: '/signin/step-up', name: 'step-up', component: () => import('../features/account/components/StepUpPage.vue') },
      { path: '/no-access', name: 'no-access', component: () => import('../features/access/components/NoAccessPage.vue') },
      { path: '/account', name: 'account', component: () => import('../features/account/components/AccountPage.vue') },
      { path: '/security', name: 'security', component: () => import('../features/security/components/SecurityPage.vue') },
      { path: '/security/password', name: 'password', component: () => import('../features/security/components/PasswordPage.vue') },
      { path: '/security/two-factor', name: 'two-factor', component: () => import('../features/security/components/TwoFactorPage.vue') },
      { path: '/security/two-factor/setup', name: 'two-factor-setup', component: () => import('../features/security/components/TwoFactorSetupPage.vue') },
      { path: '/tokens', name: 'tokens', component: () => import('../features/tokens/components/TokensPage.vue') },
      { path: '/admin', name: 'dashboard', component: () => import('../features/admin/components/DashboardPage.vue') },
      { path: '/admin/unassigned', name: 'unassigned', component: () => import('../features/access/components/UnassignedPage.vue') },
      { path: '/admin/groups/:groupId?', name: 'groups', component: () => import('../features/access/components/GroupsPage.vue') },
      { path: '/admin/apps', name: 'apps', component: () => import('../features/access/components/AppsPage.vue') },
      { path: '/admin/apps/:clientId', name: 'app', component: () => import('../features/access/components/AppPage.vue') },
      { path: '/admin/people', name: 'people', component: () => import('../features/access/components/PeoplePage.vue') },
      { path: '/admin/people/:userId', name: 'person', component: () => import('../features/access/components/PersonPage.vue') },
      { path: '/admin/audit', name: 'audit', component: () => import('../features/admin/components/AuditPage.vue') },
      // Where the pages used to be, for links already sent or bookmarked.
      { path: '/profile', redirect: '/account' },
      { path: '/profile/tokens', redirect: '/tokens' },
      { path: '/access', redirect: '/admin/groups' },
      { path: '/:rest(.*)*', redirect: '/' },
    ],
  })
}
