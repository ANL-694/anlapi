import { defineComponent, type PropType } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AppWorkspace } from '@/utils/workspace'

const userStore = vi.hoisted(() => ({
  user: { id: 1, role: 'admin' },
  isSimpleMode: false,
}))

const onboardingStore = vi.hoisted(() => ({
  getDriverInstance: vi.fn(() => null),
  setDriverInstance: vi.fn(),
  isDriverActive: vi.fn(() => false),
  setControlMethods: vi.fn(),
  clearControlMethods: vi.fn(),
}))

const getAdminSteps = vi.hoisted(() => vi.fn(() => [{ popover: { title: '管理端' } }]))
const getUserSteps = vi.hoisted(() => vi.fn(() => [{ popover: { title: '用户端' } }]))
const driverInstance = vi.hoisted(() => ({
  destroy: vi.fn(),
  drive: vi.fn(),
  isActive: vi.fn(() => false),
  getActiveIndex: vi.fn(() => 0),
  getActiveElement: vi.fn(() => null),
  moveNext: vi.fn(),
  movePrevious: vi.fn(),
}))
const createDriver = vi.hoisted(() => vi.fn(() => driverInstance))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => userStore,
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => onboardingStore,
}))

vi.mock('@/components/Guide/steps', () => ({
  getAdminSteps,
  getUserSteps,
}))

vi.mock('driver.js', () => ({
  driver: createDriver,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

import { useOnboardingTour } from '../useOnboardingTour'

const Harness = defineComponent({
  props: {
    workspace: {
      type: String as PropType<AppWorkspace>,
      required: true,
    },
  },
  setup(props) {
    return {
      tour: useOnboardingTour({
        workspace: () => props.workspace,
        autoStart: false,
      }),
    }
  },
  template: '<div />',
})

describe('useOnboardingTour workspace selection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    onboardingStore.getDriverInstance.mockReturnValue(null)
    onboardingStore.isDriverActive.mockReturnValue(false)
    driverInstance.isActive.mockReturnValue(false)
  })

  it('shows the user guide when an administrator is in the user workspace', async () => {
    const wrapper = mount(Harness, { props: { workspace: 'user' } })

    await wrapper.vm.tour.startTour()

    expect(getUserSteps).toHaveBeenCalledOnce()
    expect(getAdminSteps).not.toHaveBeenCalled()
    expect(driverInstance.drive).toHaveBeenCalledWith(0)
    wrapper.unmount()
  })

  it('shows the admin guide only in the admin workspace', async () => {
    const wrapper = mount(Harness, { props: { workspace: 'admin' } })

    await wrapper.vm.tour.startTour()

    expect(getAdminSteps).toHaveBeenCalledOnce()
    expect(getUserSteps).not.toHaveBeenCalled()
    expect(driverInstance.drive).toHaveBeenCalledWith(0)
    wrapper.unmount()
  })
})
