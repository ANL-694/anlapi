import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdminWithdrawalsView from '../AdminWithdrawalsView.vue'

const { getWithdrawals, getWithdrawal, settleWithdrawal, rejectWithdrawal } = vi.hoisted(() => ({
  getWithdrawals: vi.fn(),
  getWithdrawal: vi.fn(),
  settleWithdrawal: vi.fn(),
  rejectWithdrawal: vi.fn(),
}))

vi.mock('@/api/admin/payment', () => ({
  adminPaymentAPI: { getWithdrawals, getWithdrawal, settleWithdrawal, rejectWithdrawal },
  default: { getWithdrawals, getWithdrawal, settleWithdrawal, rejectWithdrawal },
}))

vi.mock('vue-i18n', async importOriginal => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const pendingWithdrawal = {
  id: 7,
  user_id: 9,
  user_email: 'user@example.com',
  amount: 10,
  fee_amount: 1,
  total_deducted: 11,
  balance_before: 20,
  balance_after: 9,
  payment_method: 'alipay',
  receipt_code_url: 'https://example.test/receipt.png',
  receipt_code_content_type: 'image/png',
  receipt_code_byte_size: 123,
  receipt_code_sha256: 'sha256',
  receipt_code_updated_at: '2026-08-22T00:00:00Z',
  status: 'PENDING',
  created_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
}

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `,
}

describe('AdminWithdrawalsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getWithdrawals.mockResolvedValue({ data: { items: [pendingWithdrawal], total: 1 } })
    getWithdrawal.mockResolvedValue({ data: pendingWithdrawal })
    settleWithdrawal.mockResolvedValue({ data: { ...pendingWithdrawal, status: 'SETTLED' } })
    rejectWithdrawal.mockResolvedValue({ data: { ...pendingWithdrawal, status: 'REJECTED' } })
  })

  function mountView() {
    return mount(AdminWithdrawalsView, {
      global: {
        plugins: [createPinia()],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: DataTableStub,
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
          Pagination: true,
          Select: true,
          Icon: true,
          UiIconButton: true,
        },
      },
    })
  }

  it('loads withdrawals and exposes settle/reject actions for pending rows', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(getWithdrawals).toHaveBeenCalledWith(expect.objectContaining({ page: 1, page_size: 20 }))
    expect(wrapper.text()).toContain('admin.withdrawals.settle')
    expect(wrapper.text()).toContain('admin.withdrawals.reject')
  })

  it('passes the administrator note to settle and reloads the list', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button:nth-of-type(2)').trigger('click')
    await flushPromises()
    const textarea = wrapper.get('textarea')
    await textarea.setValue('verified')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(settleWithdrawal).toHaveBeenCalledWith(7, { note: 'verified' })
    expect(getWithdrawals).toHaveBeenCalledTimes(2)
  })

  it('passes the administrator note to reject', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button:nth-of-type(3)').trigger('click')
    await flushPromises()
    await wrapper.get('textarea').setValue('invalid receipt')
    await wrapper.get('button.btn-danger').trigger('click')
    await flushPromises()

    expect(rejectWithdrawal).toHaveBeenCalledWith(7, { note: 'invalid receipt' })
  })
})
