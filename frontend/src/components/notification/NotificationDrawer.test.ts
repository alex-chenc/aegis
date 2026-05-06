// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import NotificationDrawer from './NotificationDrawer.vue'

const ElDrawerStub = {
  name: 'ElDrawer',
  template: '<div class="el-drawer-stub"><slot /><slot name="header" /></div>',
  props: ['modelValue', 'title', 'direction', 'size', 'appendToBody']
}

const ElTabsStub = {
  name: 'ElTabs',
  template: '<div class="el-tabs-stub"><slot /></div>',
  props: ['modelValue']
}

const ElTabPaneStub = {
  name: 'ElTabPane',
  template: '<div class="el-tab-pane-stub"><slot /></div>',
  props: ['label', 'name']
}

const ElEmptyStub = {
  name: 'ElEmpty',
  template: '<div class="el-empty-stub">{{ description }}</div>',
  props: ['description']
}

const ElButtonStub = {
  name: 'ElButton',
  template: '<button class="el-button-stub"><slot /></button>',
  props: ['link', 'type', 'disabled']
}

describe('NotificationDrawer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('should have append-to-body attribute on el-drawer', () => {
    const wrapper = mount(NotificationDrawer, {
      global: {
        stubs: {
          ElDrawer: ElDrawerStub,
          ElTabs: ElTabsStub,
          ElTabPane: ElTabPaneStub,
          ElEmpty: ElEmptyStub,
          ElButton: ElButtonStub
        }
      }
    })
    const drawer = wrapper.findComponent(ElDrawerStub)
    expect(drawer.exists()).toBe(true)
    expect(drawer.props('appendToBody')).toBe(true)
  })

  it('should render drawer with correct direction', () => {
    const wrapper = mount(NotificationDrawer, {
      global: {
        stubs: {
          ElDrawer: ElDrawerStub,
          ElTabs: ElTabsStub,
          ElTabPane: ElTabPaneStub,
          ElEmpty: ElEmptyStub,
          ElButton: ElButtonStub
        }
      }
    })
    const drawer = wrapper.findComponent(ElDrawerStub)
    expect(drawer.props('direction')).toBe('rtl')
  })

  it('should render drawer with correct size', () => {
    const wrapper = mount(NotificationDrawer, {
      global: {
        stubs: {
          ElDrawer: ElDrawerStub,
          ElTabs: ElTabsStub,
          ElTabPane: ElTabPaneStub,
          ElEmpty: ElEmptyStub,
          ElButton: ElButtonStub
        }
      }
    })
    const drawer = wrapper.findComponent(ElDrawerStub)
    expect(drawer.props('size')).toBe('480px')
  })

  it('should have title message notification', () => {
    const wrapper = mount(NotificationDrawer, {
      global: {
        stubs: {
          ElDrawer: ElDrawerStub,
          ElTabs: ElTabsStub,
          ElTabPane: ElTabPaneStub,
          ElEmpty: ElEmptyStub,
          ElButton: ElButtonStub
        }
      }
    })
    const drawer = wrapper.findComponent(ElDrawerStub)
    expect(drawer.props('title')).toBe('消息通知')
  })
})
