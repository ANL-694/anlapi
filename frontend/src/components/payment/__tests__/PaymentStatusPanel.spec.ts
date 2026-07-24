import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const pollOrderStatus = vi.hoisted(() => vi.fn())
const cancelOrder = vi.hoisted(() => vi.fn())
const verifyOrder = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const toCanvas = vi.hoisted(() => vi.fn())

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' },
    }),
  }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    pollOrderStatus,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    cancelOrder,
    verifyOrder,
  },
}))

vi.mock('qrcode', () => ({
  default: {
    toCanvas,
  },
}))

import PaymentStatusPanel from '../PaymentStatusPanel.vue'

const originalLocation = window.location
const originalHiddenDescriptor = Object.getOwnPropertyDescriptor(document, 'hidden')

function mockAlipayBrowser(hidden: () => boolean = () => false) {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { assign },
  })
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    get: hidden,
  })
  return assign
}

const orderFactory = (status: string) => ({
  id: 42,
  user_id: 9,
  amount: 88,
  pay_amount: 88,
  fee_rate: 0,
  payment_type: 'alipay',
  out_trade_no: 'sub2_20260420abcd1234',
  status,
  order_type: 'balance',
  created_at: '2026-04-20T12:00:00Z',
  expires_at: '2099-01-01T12:30:00Z',
  refund_amount: 0,
})

const mobileAlipayProps = {
  orderId: 42,
  amount: 88,
  payAmount: 88,
  qrCode: 'https://qr.alipay.com/dynamic-order-42',
  expiresAt: '2099-01-01T12:30:00Z',
  paymentType: 'alipay',
  orderType: 'balance',
  currency: 'CNY',
  outTradeNo: 'sub2_20260420abcd1234',
  mobileAlipayDeepLink: true,
}

function mountMobileAlipayPanel() {
  return mount(PaymentStatusPanel, {
    props: mobileAlipayProps,
    global: { stubs: { Icon: true } },
  })
}

describe('PaymentStatusPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    pollOrderStatus.mockReset()
    cancelOrder.mockReset()
    verifyOrder.mockReset()
    showError.mockReset()
    toCanvas.mockReset().mockResolvedValue(undefined)
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
    if (originalHiddenDescriptor) {
      Object.defineProperty(document, 'hidden', originalHiddenDescriptor)
    } else {
      delete (document as Document & { hidden?: boolean }).hidden
    }
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('treats RECHARGING as a successful terminal state', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory('RECHARGING'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(pollOrderStatus).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.emitted('success')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows reopen button in QR mode when payUrl is also available', async () => {
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({ closed: false } as Window)

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        payUrl: 'https://pay.example.com/session/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('payment.qr.openPayWindow')

    await wrapper.get('button.btn.btn-secondary.text-sm').trigger('click')
    expect(openSpy).toHaveBeenCalledWith(
      'https://pay.example.com/session/42',
      'paymentPopup',
      expect.any(String),
    )

    wrapper.unmount()
    openSpy.mockRestore()
  })

  it('actively verifies a pending mobile Alipay precreate order', async () => {
    mockAlipayBrowser()
    pollOrderStatus.mockResolvedValue(orderFactory('PENDING'))
    verifyOrder.mockResolvedValue({ data: orderFactory('COMPLETED') })

    const wrapper = mountMobileAlipayPanel()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(verifyOrder).toHaveBeenCalledWith('sub2_20260420abcd1234')
    expect(wrapper.emitted('success')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows the dynamic QR fallback only after the Alipay launch timeout', async () => {
    const assign = mockAlipayBrowser()
    const wrapper = mountMobileAlipayPanel()

    await flushPromises()
    expect(assign).toHaveBeenCalledWith(
      'alipays://platformapi/startapp?saId=10000007&qrcode=https%3A%2F%2Fqr.alipay.com%2Fdynamic-order-42',
    )
    expect(wrapper.find('[data-test="alipay-qr-fallback"]').exists()).toBe(false)

    await vi.advanceTimersByTimeAsync(2199)
    expect(wrapper.find('[data-test="alipay-qr-fallback"]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()

    expect(wrapper.find('[data-test="alipay-qr-fallback"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('payment.qr.alipayFallbackTitle')
    expect(wrapper.text()).toContain('sub2_20260420abcd1234')
    expect(toCanvas).toHaveBeenCalledWith(
      expect.any(HTMLCanvasElement),
      'https://qr.alipay.com/dynamic-order-42',
      expect.any(Object),
    )
    wrapper.unmount()
  })

  it.each(['visibilitychange', 'pagehide'] as const)(
    'keeps the QR fallback hidden after %s and clears timers on unmount',
    async (eventName) => {
      let hidden = false
      mockAlipayBrowser(() => hidden)
      const wrapper = mountMobileAlipayPanel()

      await flushPromises()
      if (eventName === 'visibilitychange') {
        hidden = true
        document.dispatchEvent(new Event(eventName))
      } else {
        window.dispatchEvent(new Event(eventName))
      }
      await vi.advanceTimersByTimeAsync(2200)
      await flushPromises()

      expect(wrapper.find('[data-test="alipay-qr-fallback"]').exists()).toBe(false)
      expect(wrapper.get('[data-test="alipay-deep-link-status"]').text()).toBe('payment.qr.alipayContinueInApp')
      expect(wrapper.find('[data-test="reopen-alipay"]').exists()).toBe(true)

      wrapper.unmount()
      expect(vi.getTimerCount()).toBe(0)
    },
  )

  it('saves the fallback QR code with the order number', async () => {
    mockAlipayBrowser()
    const toDataURL = vi
      .spyOn(HTMLCanvasElement.prototype, 'toDataURL')
      .mockReturnValue('data:image/png;base64,dynamic-qr')
    let downloadedFile = ''
    let downloadedHref = ''
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      downloadedFile = this.download
      downloadedHref = this.href
    })
    const wrapper = mountMobileAlipayPanel()

    await vi.advanceTimersByTimeAsync(2200)
    await flushPromises()
    await wrapper.get('[data-test="save-alipay-qr"]').trigger('click')

    expect(wrapper.text()).toContain('payment.qr.saveQRCode')
    expect(toDataURL).toHaveBeenCalledWith('image/png')
    expect(downloadedFile).toBe('alipay-sub2_20260420abcd1234.png')
    expect(downloadedHref).toBe('data:image/png;base64,dynamic-qr')
    wrapper.unmount()
  })
})
