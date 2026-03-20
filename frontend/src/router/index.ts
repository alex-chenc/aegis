import { createRouter, createWebHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import Settings from '../views/Settings.vue'
import Workbench from '../views/Workbench.vue'
import TaskCenter from '../views/TaskCenter.vue'
import TaskDetail from '../views/TaskDetail.vue'
import Vulnerability from '../views/Vulnerability.vue'
import DetectionOverview from '../views/detection/Overview.vue'
import DetectionAlerts from '../views/detection/Alerts.vue'
import DetectionPolicies from '../views/detection/Policies.vue'
import DetectionRules from '../views/detection/Rules.vue'

const routes = [
  {
    path: '/',
    redirect: '/hosts'
  },
  {
    path: '/hosts',
    name: 'Hosts',
    component: Dashboard,
    meta: { title: '主机列表' }
  },
  {
    path: '/baseline',
    redirect: '/baseline/workbench',
    meta: { title: '智能基线检查与修复' }
  },
  {
    path: '/baseline/workbench',
    name: 'BaselineWorkbench',
    component: Workbench,
    meta: { title: '基线工作台' }
  },
  {
    path: '/baseline/tasks',
    name: 'BaselineTasks',
    component: TaskCenter,
    meta: { title: '基线任务中心' }
  },
  {
    path: '/baseline/tasks/:id',
    name: 'BaselineTaskDetail',
    component: TaskDetail,
    meta: { title: '任务详情' }
  },
  {
    path: '/vulnerability',
    name: 'Vulnerability',
    component: Vulnerability,
    meta: { title: '智能漏洞检查与修复' }
  },
  {
    path: '/vulnerability/tasks',
    name: 'VulnerabilityTasks',
    component: TaskCenter,
    meta: { title: '漏洞任务中心' }
  },
  {
    path: '/vulnerability/tasks/:id',
    name: 'VulnerabilityTaskDetail',
    component: TaskDetail,
    meta: { title: '漏洞任务详情' }
  },
  {
    path: '/detection/overview',
    name: 'DetectionOverview',
    component: DetectionOverview,
    meta: { title: '安全概览' }
  },
  {
    path: '/detection/alerts',
    name: 'DetectionAlerts',
    component: DetectionAlerts,
    meta: { title: '告警列表' }
  },
  {
    path: '/detection/policies',
    name: 'DetectionPolicies',
    component: DetectionPolicies,
    meta: { title: '阻断策略' }
  },
  {
    path: '/detection/rules',
    name: 'DetectionRules',
    component: DetectionRules,
    meta: { title: '规则管理' }
  },
  {
    path: '/settings',
    name: 'Settings',
    component: Settings,
    meta: { title: '系统配置' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
