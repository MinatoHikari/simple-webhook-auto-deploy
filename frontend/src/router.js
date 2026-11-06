import { createRouter, createWebHistory } from 'vue-router'
import LoginView from './views/LoginView.vue'
import ProjectsView from './views/ProjectsView.vue'
import ProjectDetailView from './views/ProjectDetailView.vue'
import SystemLogsView from './views/SystemLogsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginView },
    { path: '/', component: ProjectsView },
    { path: '/projects/:name', component: ProjectDetailView, props: true },
    { path: '/logs', component: SystemLogsView }
  ]
})

export default router
