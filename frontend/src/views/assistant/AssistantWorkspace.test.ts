// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AssistantWorkspace from './AssistantWorkspace.vue'

const routerMocks = vi.hoisted(() => ({
  replace: vi.fn(),
}))

const apiMocks = vi.hoisted(() => ({
  getSessions: vi.fn(),
  createSession: vi.fn(),
  getSession: vi.fn(),
  getMessages: vi.fn(),
  sendMessage: vi.fn(),
  cancelRun: vi.fn(),
  getContextRefs: vi.fn(),
  getToolCalls: vi.fn(),
  getApprovals: vi.fn(),
  approveApproval: vi.fn(),
  rejectApproval: vi.fn(),
  createAssistantStream: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => routerMocks,
}))

vi.mock('@/utils/auth', () => ({
  getStoredAuth: () => ({ token: 'test-token', username: 'admin' }),
  getAuthToken: () => 'test-token',
  clearStoredAuth: vi.fn(),
}))

vi.mock('@/api/assistant', () => apiMocks)

const AssistantSessionSidebarStub = defineComponent({
  name: 'AssistantSessionSidebar',
  props: {
    sessions: { type: Array, default: () => [] },
    activeSessionId: String,
    loading: Boolean,
    loadingMore: Boolean,
    hasMore: Boolean,
    total: Number,
    currentPage: Number,
  },
  setup(props) {
    return () =>
      h('aside', {
        class: 'assistant-session-sidebar-stub',
        'data-total': String(props.total),
        'data-has-more': String(props.hasMore),
      })
  },
})

const AssistantComposerStub = defineComponent({
  name: 'AssistantComposer',
  props: {
    disabled: Boolean,
  },
  emits: ['send'],
  setup() {
    return () => h('div', { class: 'assistant-composer-stub' })
  },
})

const PassThroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const sessions = Array.from({ length: 10 }, (_, index) => ({
  id: `id-${index}`,
  session_id: `session-${index}`,
  title: `Session ${index}`,
  task_type: 'explanation',
  status: 'active',
  message_count: 0,
  tool_call_count: 0,
  approval_count: 0,
  created_by: 'admin',
  created_at: '2026-06-07T00:00:00Z',
  updated_at: '2026-06-07T00:00:00Z',
}))

function mountWorkspace() {
  return mount(AssistantWorkspace, {
    global: {
      stubs: {
        AssistantSessionSidebar: AssistantSessionSidebarStub,
        AssistantConversation: PassThroughStub,
        AssistantComposer: AssistantComposerStub,
        AssistantContextRail: PassThroughStub,
        ContextBudgetIndicator: defineComponent({ setup: () => () => h('div', { class: 'context-budget-stub' }) }),
        'el-button': PassThroughStub,
        'el-icon': PassThroughStub,
        'el-tag': PassThroughStub,
      },
    },
  })
}

describe('AssistantWorkspace session pagination', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()

    const sessionFixtures = sessions.map(session => ({ ...session }))
    apiMocks.getSessions.mockResolvedValue({ sessions: sessionFixtures, total: 12 })
    apiMocks.getSession.mockImplementation((sessionId: string) =>
      Promise.resolve(sessionFixtures.find(session => session.session_id === sessionId) || sessionFixtures[0])
    )
    apiMocks.getMessages.mockResolvedValue([])
    apiMocks.getContextRefs.mockResolvedValue([])
    apiMocks.getToolCalls.mockResolvedValue({ items: [], total: 0 })
    apiMocks.getApprovals.mockResolvedValue({ items: [], total: 0 })
    apiMocks.createAssistantStream.mockReturnValue({ close: vi.fn() })
    apiMocks.createSession.mockResolvedValue({
      id: 'new-id',
      session_id: 'new-session',
      title: '帮我排查一下192',
      task_type: 'explanation',
      status: 'active',
      message_count: 0,
      tool_call_count: 0,
      approval_count: 0,
      created_by: 'admin',
      created_at: '2026-06-07T00:00:00Z',
      updated_at: '2026-06-07T00:00:00Z',
    })
    apiMocks.sendMessage.mockResolvedValue({ run_id: 'run-1', message_id: 'msg-user-1' })
  })

  it('passes the fetched session total to the sidebar for pagination rendering', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    const sidebar = wrapper.findComponent(AssistantSessionSidebarStub)
    expect(sidebar.props('total')).toBe(12)
    expect(sidebar.props('hasMore')).toBe(true)
  })

  it('keeps the session search keyword when changing pages', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    const sidebar = wrapper.findComponent(AssistantSessionSidebarStub)
    apiMocks.getSessions.mockClear()

    sidebar.vm.$emit('search', 'pgsql')
    await flushPromises()

    expect(apiMocks.getSessions).toHaveBeenCalledWith(expect.objectContaining({
      keyword: 'pgsql',
      page: 1,
      page_size: 10,
    }))

    apiMocks.getSessions.mockClear()
    sidebar.vm.$emit('pageChange', 2)
    await flushPromises()

    expect(apiMocks.getSessions).toHaveBeenCalledWith(expect.objectContaining({
      keyword: 'pgsql',
      page: 2,
      page_size: 10,
    }))
  })

  it('creates a new session without duplicating the first user message', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findComponent(AssistantSessionSidebarStub).vm.$emit('create')
    await flushPromises()

    wrapper.findComponent(AssistantComposerStub).vm.$emit('send', '帮我排查一下192.168.152.159 这个机器上面有哪些安全问题')
    await flushPromises()

    expect(apiMocks.createSession).toHaveBeenCalledWith(expect.not.objectContaining({
      initial_message: expect.any(String),
    }))
    expect(apiMocks.createSession).toHaveBeenCalledWith(expect.objectContaining({
      title: expect.stringContaining('帮我排查一下192'),
      task_type: 'explanation',
    }))
    expect(apiMocks.sendMessage).toHaveBeenCalledTimes(1)
    expect(apiMocks.sendMessage).toHaveBeenCalledWith('new-session', expect.objectContaining({
      content: '帮我排查一下192.168.152.159 这个机器上面有哪些安全问题',
    }))
  })

  it('reconnects the stream when opening a running session after refresh', async () => {
    apiMocks.getSession.mockResolvedValue({
      ...sessions[0],
      status: 'running',
      metadata: {
        max_total_turns: 500,
        total_prompt_tokens: 1200,
        total_completion_tokens: 300,
      },
    })

    const wrapper = mountWorkspace()
    await flushPromises()

    expect(apiMocks.createAssistantStream).toHaveBeenCalledWith('session-0')
    expect(wrapper.text()).toContain('最大轮数 500')
    expect(wrapper.text()).toContain('Tokens 1.5K')
  })
})
