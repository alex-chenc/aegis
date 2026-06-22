import { createRouter, createWebHistory } from 'vue-router'
import { getStoredAuth } from '@/utils/auth'
import Login from '../views/Login.vue'
import ForcePasswordChange from '../views/ForcePasswordChange.vue'
import Dashboard from '../views/Dashboard.vue'
import ModelSettings from '../views/settings/ModelSettings.vue'
import AgentInstall from '../views/settings/AgentInstall.vue'
import Workbench from '../views/Workbench.vue'
import TaskCenter from '../views/TaskCenter.vue'
import TaskDetail from '../views/TaskDetail.vue'
import Vulnerability from '../views/Vulnerability.vue'
import DetectionOverview from '../views/detection/Overview.vue'
import DetectionAlerts from '../views/detection/Alerts.vue'
import DetectionPolicies from '../views/detection/Policies.vue'
import DetectionRules from '../views/detection/Rules.vue'
import AIAnalysis from '../views/detection/AIAnalysis.vue'
import CommandAudit from '../views/settings/CommandAudit/index.vue'
import AuditLogs from '../views/settings/AuditLogs/index.vue'
import DetectionPackages from '../views/detection/DetectionPackages/index.vue'
import PackageDetail from '../views/detection/DetectionPackages/PackageDetail.vue'
import PackageEditor from '../views/detection/DetectionPackages/PackageEditor.vue'
import EBPFHooks from '../views/settings/EBPFHooks/index.vue'
import WeakPasswordIndex from '../views/detection/WeakPassword/Index.vue'
import WeakPasswordTaskDetail from '../views/detection/WeakPassword/TaskDetail.vue'
import WeakPasswordDictionaries from '../views/detection/WeakPassword/Dictionaries.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
    meta: { title: '登录认证', public: true, authLayout: true }
  },
  {
    path: '/force-password-change',
    name: 'ForcePasswordChange',
    component: ForcePasswordChange,
    meta: { title: '设置管理员凭据', authLayout: true, requiresAuth: true }
  },
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
  // V5.8 智能资产采集
  {
    path: '/hosts/assets',
    name: 'AssetsOverview',
    component: () => import('../views/hosts/Assets/Overview.vue'),
    meta: { title: '智能资产采集' }
  },
  {
    path: '/hosts/assets/software',
    name: 'AssetsSoftware',
    component: () => import('../views/hosts/Assets/Software.vue'),
    meta: { title: '软件清单' }
  },
  {
    path: '/hosts/assets/applications',
    name: 'AssetsApplications',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: '应用资产' }
  },
  {
    path: '/hosts/assets/databases',
    name: 'AssetsDatabases',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: '数据库资产' },
    props: { defaultCategory: 'database' }
  },
  {
    path: '/hosts/assets/web-services',
    name: 'AssetsWebServices',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: 'Web 服务资产' },
    props: { defaultCategory: 'web_service' }
  },
  {
    path: '/hosts/assets/web-frameworks',
    name: 'AssetsWebFrameworks',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: 'Web 框架资产' },
    props: { defaultCategory: 'web_framework' }
  },
  {
    path: '/hosts/assets/web-sites',
    name: 'AssetsWebSites',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: 'Web 站点资产' },
    props: { defaultCategory: 'web_site' }
  },
  {
    path: '/hosts/assets/llm-services',
    name: 'AssetsLLMServices',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: 'AI LLM 资产' },
    props: { defaultCategory: 'llm_service' }
  },
  {
    path: '/hosts/assets/ai-agents',
    name: 'AssetsAIAgents',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: 'AI Agent 资产' },
    props: { defaultCategory: 'ai_agent' }
  },
  {
    path: '/hosts/assets/mcp-servers',
    name: 'AssetsMCPServers',
    component: () => import('../views/hosts/Assets/Applications.vue'),
    meta: { title: 'MCP 资产' },
    props: { defaultCategory: 'mcp_server' }
  },
  {
    path: '/hosts/assets/collections',
    name: 'AssetsCollections',
    redirect: '/hosts/assets',
    meta: { title: '采集任务' }
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
    path: '/detection/ai-analysis',
    name: 'AIAnalysis',
    component: AIAnalysis,
    meta: { title: 'AI 分析' }
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
    path: '/detection/packages',
    name: 'DetectionPackages',
    component: DetectionPackages,
    meta: { title: '动态检测包' }
  },
  {
    path: '/detection/packages/new',
    name: 'DetectionPackageNew',
    component: PackageEditor,
    meta: { title: '新建检测包' }
  },
  {
    path: '/detection/packages/:id',
    name: 'DetectionPackageDetail',
    component: PackageDetail,
    meta: { title: '检测包详情' }
  },
  {
    path: '/detection/packages/:id/edit',
    name: 'DetectionPackageEdit',
    component: PackageEditor,
    meta: { title: '编辑检测包' }
  },
  {
    path: '/risk/weak-password',
    name: 'WeakPassword',
    component: WeakPasswordIndex,
    meta: { title: '智能弱密码检测' }
  },
  {
    path: '/risk/weak-password/tasks/:id',
    name: 'WeakPasswordTaskDetail',
    component: WeakPasswordTaskDetail,
    meta: { title: '智能弱密码任务详情' }
  },
  {
    path: '/risk/weak-password/dictionaries',
    name: 'WeakPasswordDictionaries',
    component: WeakPasswordDictionaries,
    meta: { title: '弱密码字典' }
  },
  {
    path: '/settings/command-audit',
    name: 'CommandAudit',
    component: CommandAudit,
    meta: { title: '命令审计配置' }
  },
  {
    path: '/settings/audit-logs',
    name: 'AuditLogs',
    component: AuditLogs,
    meta: { title: '审计日志' }
  },
  {
    path: '/settings',
    redirect: '/settings/models',
    meta: { title: '系统配置' }
  },
  {
    path: '/settings/models',
    name: 'ModelSettings',
    component: ModelSettings,
    meta: { title: '模型配置' }
  },
  {
    path: '/settings/agent',
    name: 'AgentInstall',
    component: AgentInstall,
    meta: { title: 'Agent 安装' }
  },
  {
    path: '/settings/ebpf-hooks',
    name: 'EBPFHooks',
    component: EBPFHooks,
    meta: { title: 'eBPF Hook 白名单' }
  },
  {
    path: '/settings/tool-policy',
    name: 'ToolPolicySettings',
    component: () => import('../views/settings/AssistantToolPolicySettings.vue'),
    meta: { title: '智能体工具权限' }
  },
  // V6.0 智能助手
  {
    path: '/assistant',
    name: 'Assistant',
    component: () => import('../views/assistant/AssistantWorkspace.vue'),
    meta: { title: '智能助手', requiresAuth: true }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to) => {
  const auth = getStoredAuth()

  if (to.path === '/login' && auth) {
    return auth.forcePasswordChange ? '/force-password-change' : '/hosts'
  }

  if (!to.meta.public && !auth) {
    return '/login'
  }

  if (auth?.forcePasswordChange && to.path !== '/force-password-change') {
    return '/force-password-change'
  }

  if (to.path === '/force-password-change' && auth && !auth.forcePasswordChange) {
    return '/hosts'
  }

  return true
})

export default router
