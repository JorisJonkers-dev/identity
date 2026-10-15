import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { configureApi } from '../../../infrastructure/http'
import { describeError, useNotes } from './useNotes'

describe('useNotes', () => {
  it('has no notes before the first response', () => {
    // A request that never settles keeps the query pending for the whole test.
    configureApi({ baseUrl: 'http://app.test', fetch: () => new Promise<Response>(() => undefined) })
    let notes: ReturnType<typeof useNotes> | undefined
    mount(defineComponent({ setup: () => ((notes = useNotes()), () => h('div')) }), {
      global: { plugins: [[VueQueryPlugin, { queryClient: new QueryClient() }]] },
    })
    expect(notes?.notes.value).toEqual([])
    expect(notes?.isLoading.value).toBe(true)
  })
})

describe('describeError', () => {
  it("prefers a problem's detail, then its title", () => {
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400, detail: 'Fix it.' })).toBe('Fix it.')
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400 })).toBe('Bad Request')
  })

  it('falls back for anything that is not a problem', () => {
    expect(describeError(new TypeError('Failed to fetch'))).toBe('Something went wrong. Try again.')
  })
})
