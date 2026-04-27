// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import ForcePasswordChange from './ForcePasswordChange.vue'

const { replaceMock, changeCredentialsMock, saveAuthSessionMock } = vi.hoisted(() => ({
  replaceMock: vi.fn(),
  changeCredentialsMock: vi.fn(),
  saveAuthSessionMock: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    replace: replaceMock
  })
}))

vi.mock('@/api/auth', () => ({
  changeCredentials: changeCredentialsMock
}))

vi.mock('@/utils/auth', () => ({
  saveAuthSession: saveAuthSessionMock
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual<typeof import('element-plus')>('element-plus')
  return {
    ...actual,
    ElMessage: {
      success: vi.fn()
    }
  }
})

const ButtonStub = defineComponent({
  name: 'ElButtonStub',
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

describe('ForcePasswordChange view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('submits new credentials and redirects to hosts', async () => {
    changeCredentialsMock.mockResolvedValueOnce({
      token: 'token-2',
      username: 'security-admin',
      force_password_change: false
    })

    const wrapper = mount(ForcePasswordChange, {
      global: {
        stubs: {
          'el-button': ButtonStub,
          'el-input': InputStub,
          'el-form': FormStub,
          'el-form-item': PassThroughStub
        }
      }
    })

    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('security-admin')
    await inputs[1].setValue('StrongerPassword123!')
    await inputs[2].setValue('StrongerPassword123!')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(changeCredentialsMock).toHaveBeenCalledWith({
      username: 'security-admin',
      new_password: 'StrongerPassword123!',
      confirm_password: 'StrongerPassword123!'
    })
    expect(saveAuthSessionMock).toHaveBeenCalledWith({
      token: 'token-2',
      username: 'security-admin',
      force_password_change: false
    })
    expect(replaceMock).toHaveBeenCalledWith('/hosts')
  })
})
