// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import Login from './Login.vue'

const {
  replaceMock,
  getAuthStatusMock,
  bootstrapLoginMock,
  loginMock,
  saveAuthSessionMock
} = vi.hoisted(() => ({
  replaceMock: vi.fn(),
  getAuthStatusMock: vi.fn(),
  bootstrapLoginMock: vi.fn(),
  loginMock: vi.fn(),
  saveAuthSessionMock: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    replace: replaceMock
  })
}))

vi.mock('@/api/auth', () => ({
  getAuthStatus: getAuthStatusMock,
  bootstrapLogin: bootstrapLoginMock,
  login: loginMock
}))

vi.mock('@/utils/auth', () => ({
  saveAuthSession: saveAuthSessionMock
}))

const ButtonStub = defineComponent({
  name: 'ElButtonStub',
  props: ['loading'],
  emits: ['click'],
  setup(_, { slots, emit }) {
    return () => h('button', { onClick: () => emit('click') }, slots.default?.())
  }
})

const InputStub = defineComponent({
  name: 'ElInputStub',
  inheritAttrs: false,
  props: ['modelValue'],
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        value: props.modelValue,
        onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value)
      })
  }
})

const FormStub = defineComponent({
  name: 'ElFormStub',
  setup(_, { slots, expose }) {
    expose({ validate: () => Promise.resolve(true) })
    return () => h('form', slots.default?.())
  }
})

const PassThroughStub = defineComponent({
  name: 'PassThroughStub',
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  }
})

function mountLogin() {
  return mount(Login, {
    global: {
      stubs: {
        'el-button': ButtonStub,
        'el-input': InputStub,
        'el-form': FormStub,
        'el-form-item': PassThroughStub,
        'el-icon': PassThroughStub,
        'el-alert': PassThroughStub,
        'el-skeleton': PassThroughStub
      }
    }
  })
}

async function settle() {
  await flushPromises()
  await nextTick()
}

describe('Login view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows bootstrap action when auth is not initialized', async () => {
    getAuthStatusMock.mockResolvedValueOnce({ initialized: false })
    const wrapper = mountLogin()
    await settle()

    expect(wrapper.text()).toContain('首次进入控制台')
  })

  it('saves temporary session and redirects to forced password change', async () => {
    getAuthStatusMock.mockResolvedValueOnce({ initialized: false })
    bootstrapLoginMock.mockResolvedValueOnce({
      token: 'token-1',
      username: 'admin',
      force_password_change: true
    })
    const wrapper = mountLogin()
    await settle()

    const button = wrapper.find('button')
    await button!.trigger('click')
    await settle()

    expect(saveAuthSessionMock).toHaveBeenCalledWith({
      token: 'token-1',
      username: 'admin',
      force_password_change: true
    })
    expect(replaceMock).toHaveBeenCalledWith('/force-password-change')
  })
})
