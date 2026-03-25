import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    name: 'Auth',
    component: () => import('../views/AuthView.vue')
  },
  {
    path: '/vault',
    name: 'Vault',
    component: () => import('../views/VaultView.vue')
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('../views/SettingsView.vue')
  },
  {
    path: '/steganography',
    name: 'Steganography',
    component: () => import('../views/StegoView.vue')
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router
