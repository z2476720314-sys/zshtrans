import test from 'node:test'
import assert from 'node:assert/strict'
import { api } from '../src/api.ts'

test('translation passes cancellation to the network request', async () => {
  const original = globalThis.fetch
  const controller = new AbortController()
  globalThis.fetch = async (_url, init) => {
    assert.equal(init.signal, controller.signal)
    return new Response(JSON.stringify({ text: '订单已发出', provider: 'test', ms: 1, src: 'ru', tgt: 'zh' }))
  }
  try {
    const result = await api.translate('Заказ отправлен', 'ru', 'zh', controller.signal)
    assert.equal(result.text, '订单已发出')
  } finally {
    globalThis.fetch = original
  }
})

test('selection preserves detected and target language in its response', async () => {
  const original = globalThis.fetch
  const controller = new AbortController()
  globalThis.fetch = async (_url, init) => {
    assert.equal(init.signal, controller.signal)
    return new Response(JSON.stringify({ source: 'Заказ отправлен', text: '订单已发出', src: 'ru', tgt: 'zh', provider: 'test', ms: 1 }))
  }
  try {
    const result = await api.selection(controller.signal)
    assert.equal(result.src, 'ru')
    assert.equal(result.tgt, 'zh')
  } finally {
    globalThis.fetch = original
  }
})
