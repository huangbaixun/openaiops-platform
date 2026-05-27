import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'home', component: () => import('../views/HomeView.vue') },
        {
          path: 'traces',
          name: 'traces',
          component: () => import('../views/Traces/TracesList.vue'),
        },
        {
          path: 'traces/:traceId',
          name: 'trace-detail',
          component: () => import('../views/Traces/TraceDetail.vue'),
          props: true,
        },
        {
          path: 'logs',
          name: 'logs',
          component: () => import('../views/Logs/LogsView.vue'),
        },
        {
          path: 'overview',
          name: 'overview',
          component: () => import('../views/Overview/OverviewPage.vue'),
        },
        {
          path: 'services/:name',
          name: 'service-detail',
          component: () => import('../views/Services/ServiceDetail.vue'),
          props: true,
        },
        {
          path: 'topology',
          name: 'topology',
          component: () => import('../views/Topology/TopologyPage.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.isAuthenticated) {
    await auth.restore()
  }
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login' }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'home' }
  }
})

export default router
