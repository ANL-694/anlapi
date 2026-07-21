import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import BackupView from '../BackupView.vue'

const mockGetImageStorageConfig = vi.fn()
const mockUpdateImageStorageConfig = vi.fn()
const mockTestImageStorageConnection = vi.fn()
const mockGetFileCardStorage = vi.fn()
const mockGetSettings = vi.fn()

vi.mock('@/api', () => ({
  adminAPI: {
    backup: {
      getS3Config: vi.fn().mockResolvedValue({}),
      updateS3Config: vi.fn(),
      testS3Connection: vi.fn(),
      getImageStorageConfig: (...args: unknown[]) => mockGetImageStorageConfig(...args),
      updateImageStorageConfig: (...args: unknown[]) => mockUpdateImageStorageConfig(...args),
      testImageStorageConnection: (...args: unknown[]) => mockTestImageStorageConnection(...args),
      getSchedule: vi.fn().mockResolvedValue({}),
      updateSchedule: vi.fn(),
      getUsageRetention: vi.fn().mockResolvedValue({}),
      updateUsageRetention: vi.fn(),
      listBackups: vi.fn().mockResolvedValue({ items: [] }),
      getBackup: vi.fn(),
      createBackup: vi.fn(),
      getDownloadURL: vi.fn(),
      restoreBackup: vi.fn(),
      deleteBackup: vi.fn(),
    },
    settings: {
      getSettings: (...args: unknown[]) => mockGetSettings(...args),
      updateSettings: vi.fn(),
    },
  },
}))

vi.mock('@/api/admin/store', () => ({
  adminStoreAPI: {
    getFileCardStorage: (...args: unknown[]) => mockGetFileCardStorage(...args),
    updateFileCardStorage: vi.fn(),
    testFileCardStorage: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showWarning: vi.fn() }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (action: () => unknown) => action() }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('BackupView async image storage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetImageStorageConfig.mockResolvedValue({
      config: {
        enabled: false,
        reuse_backup_s3: true,
        bucket: '',
        prefix: 'images/',
        public_base_url: '',
        presign_expiry_hours: 24,
        max_download_bytes: 32 * 1024 * 1024,
        endpoint: '',
        region: 'auto',
        access_key_id: '',
        force_path_style: false,
      },
      secret_configured: false,
    })
    mockUpdateImageStorageConfig.mockResolvedValue({})
    mockTestImageStorageConnection.mockResolvedValue({ ok: true, message: 'ok' })
    mockGetFileCardStorage.mockResolvedValue({ data: {} })
    mockGetSettings.mockResolvedValue({})
  })

  it('加载异步生图设置，并在保存时只提交该配置', async () => {
    const wrapper = mount(BackupView, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    expect(mockGetImageStorageConfig).toHaveBeenCalledTimes(1)
    expect(mockGetFileCardStorage).toHaveBeenCalledTimes(1)
    expect(mockGetSettings).toHaveBeenCalledTimes(1)

    const imageStorageCard = wrapper.findAll('.card').find(card =>
      card.text().includes('admin.backup.imageStorage.title'),
    )
    expect(imageStorageCard).toBeDefined()
    const enabled = imageStorageCard!.find('input[type="checkbox"]')
    await enabled.setValue(true)
    const save = imageStorageCard!.findAll('button').find(button => button.text() === 'common.save')
    expect(save).toBeDefined()
    await save!.trigger('click')
    await flushPromises()

    expect(mockUpdateImageStorageConfig).toHaveBeenCalledWith(expect.objectContaining({
      enabled: true,
      reuse_backup_s3: true,
      prefix: 'images/',
    }))
  })
})
