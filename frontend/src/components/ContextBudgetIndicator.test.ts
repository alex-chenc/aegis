// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ContextBudgetIndicator from './ContextBudgetIndicator.vue'

describe('ContextBudgetIndicator', () => {
  it('falls back to total prompt tokens when the budget snapshot is still preflight-sized', () => {
    const wrapper = mount(ContextBudgetIndicator, {
      props: {
        budget: {
          max_context_tokens: 10000,
          reserved_output_tokens: 0,
          estimated_prompt_tokens: 32,
          context_ratio: 0.0032,
          prompt_tokens_observed: 0,
          completion_tokens: 0,
          total_tokens: 0,
          compression_count: 0,
        },
        totalPromptTokens: 4096,
      },
      global: {
        stubs: {
          ElPopover: {
            template: '<div><slot name="reference" /><slot /></div>',
          },
        },
      },
    })

    expect(wrapper.find('.ring-label').text()).toBe('41%')
    expect(wrapper.text()).toContain('4.1K')
  })
})
