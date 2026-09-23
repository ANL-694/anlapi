import { beforeEach, describe, expect, it } from 'vitest'
import { updateFavicon } from '@/utils/branding'

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
    expect(link?.type).toBe('image/png')
  })

  it('detects image MIME types from data URLs and query-string URLs', () => {
    updateFavicon('https://example.com/custom-logo.webp?v=2')
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.type).toBe('image/webp')

    updateFavicon('data:image/png;base64,abc')
    expect(link?.type).toBe('image/png')
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})
