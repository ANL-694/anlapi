import { sanitizeUrl } from '@/utils/url'

export const DEFAULT_BRAND_NAME = 'ANL Gateway'
export const DEFAULT_FAVICON = '/logo.svg'

function faviconMimeType(logoUrl: string): string {
  const dataMime = logoUrl.match(/^data:([^;,\s]+)/i)?.[1]
  if (dataMime?.startsWith('image/')) return dataMime

  const path = logoUrl.split(/[?#]/, 1)[0].toLowerCase()
  if (path.endsWith('.png')) return 'image/png'
  if (path.endsWith('.jpg') || path.endsWith('.jpeg')) return 'image/jpeg'
  if (path.endsWith('.webp')) return 'image/webp'
  if (path.endsWith('.gif')) return 'image/gif'
  if (path.endsWith('.svg')) return 'image/svg+xml'
  return 'image/x-icon'
}

export function updateFavicon(logoUrl?: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl || '', {
    allowRelative: true,
    allowDataUrl: true,
  }) || DEFAULT_FAVICON

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = faviconMimeType(sanitizedLogoUrl)
  link.href = sanitizedLogoUrl
}
