export function normalizeEndpointUrl(endpoint: string): string {
  const trimmed = endpoint.trim()
  if (!trimmed) return ''
  const protocolRelative = trimmed.startsWith('//')
  const relative = trimmed.startsWith('/') && !protocolRelative
  const withScheme = protocolRelative
    ? `https:${trimmed}`
    : relative || /^[a-z][a-z\d+\-.]*:\/\//i.test(trimmed)
      ? trimmed
      : `https://${trimmed}`

  try {
    const parsed = new URL(withScheme, 'http://sub2api.local')
    if (!['http:', 'https:'].includes(parsed.protocol)) return ''
    parsed.pathname = parsed.pathname.replace(/\/+$/, '') || '/'
    if (relative) return `${parsed.pathname}${parsed.search}${parsed.hash}`
    const normalized = parsed.toString()
    return parsed.pathname === '/' && !parsed.search && !parsed.hash
      ? normalized.replace(/\/$/, '')
      : normalized
  } catch {
    return ''
  }
}

export function appendEndpointPath(endpoint: string, suffix: string): string {
  const normalized = normalizeEndpointUrl(endpoint)
  if (!normalized) return ''

  const relative = normalized.startsWith('/') && !normalized.startsWith('//')
  try {
    const parsed = new URL(normalized, 'http://sub2api.local')
    const cleanSuffix = suffix.replace(/^\/+/, '')
    const basePath = parsed.pathname.replace(/\/+$/, '')
    parsed.pathname = `${basePath}/${cleanSuffix}`.replace(/\/+/g, '/')
    if (relative) return `${parsed.pathname}${parsed.search}${parsed.hash}`
    return parsed.toString()
  } catch {
    return ''
  }
}

export function endpointKey(endpoint: string): string {
  const normalized = normalizeEndpointUrl(endpoint)
  if (!normalized || normalized.startsWith('/')) return normalized

  try {
    const url = new URL(normalized)
    url.protocol = url.protocol.toLowerCase()
    url.hostname = url.hostname.toLowerCase()
    return url.toString().replace(/\/$/, '')
  } catch {
    return normalized
  }
}
