/**
 * Vitest 测试环境设置
 * 提供全局 mock 和测试工具
 */
import { config } from '@vue/test-utils'
import { afterEach, beforeEach, vi } from 'vitest'

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()

  return {
    get length() {
      return values.size
    },
    clear: vi.fn(() => {
      values.clear()
    }),
    getItem: vi.fn((key: string) => {
      return values.has(key) ? values.get(key)! : null
    }),
    key: vi.fn((index: number) => {
      return Array.from(values.keys())[index] ?? null
    }),
    removeItem: vi.fn((key: string) => {
      values.delete(key)
    }),
    setItem: vi.fn((key: string, value: string) => {
      values.set(key, String(value))
    })
  }
}

function hasStorageApi(value: unknown): value is Storage {
  if (!value || typeof value !== 'object') return false

  const storage = value as Partial<Storage>
  return ['getItem', 'setItem', 'removeItem', 'clear', 'key'].every(
    (method) => typeof storage[method as keyof Storage] === 'function'
  ) && typeof storage.length === 'number'
}

function installStorageMocks() {
  const localStorage = createMemoryStorage()
  const sessionStorage = createMemoryStorage()

  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: localStorage
  })
  Object.defineProperty(globalThis, 'sessionStorage', {
    configurable: true,
    value: sessionStorage
  })

  if (typeof window !== 'undefined') {
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: localStorage
    })
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      value: sessionStorage
    })
  }
}

if (!hasStorageApi(globalThis.localStorage) || !hasStorageApi(globalThis.sessionStorage)) {
  installStorageMocks()
}

// Mock requestIdleCallback (Safari < 15 不支持)
if (typeof globalThis.requestIdleCallback === 'undefined') {
  globalThis.requestIdleCallback = ((callback: IdleRequestCallback) => {
    return window.setTimeout(() => callback({ didTimeout: false, timeRemaining: () => 50 }), 1)
  }) as unknown as typeof requestIdleCallback
}

if (typeof globalThis.cancelIdleCallback === 'undefined') {
  globalThis.cancelIdleCallback = ((id: number) => {
    window.clearTimeout(id)
  }) as unknown as typeof cancelIdleCallback
}

// Mock matchMedia (jsdom 未实现;DataTable 等组件依赖它做桌面/移动分支)
function installBrowserMocks() {
  if (typeof window !== 'undefined') {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: (query: string) => ({
        matches: true, // 测试默认按桌面视口渲染表格
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }) as MediaQueryList
    })
  }

  if (typeof navigator !== 'undefined') {
    Object.defineProperty(navigator, 'language', {
      configurable: true,
      value: 'en-US'
    })
  }
}

installBrowserMocks()
beforeEach(() => {
  installStorageMocks()
  installBrowserMocks()
  if (typeof window !== 'undefined') {
    delete window.__APP_CONFIG__
  }
})
afterEach(() => {
  vi.useRealTimers()
})

// Mock IntersectionObserver
class MockIntersectionObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
}

globalThis.IntersectionObserver = MockIntersectionObserver as unknown as typeof IntersectionObserver

// Mock ResizeObserver
class MockResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
}

globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

// Vue Test Utils 全局配置
config.global.stubs = {
  // 可以在这里添加全局 stub
}

// 设置全局测试超时
vi.setConfig({ testTimeout: 10000 })
