/**
 * formatScaled formats a per-token (or per-request) USD price scaled by `scale`.
 *
 *   formatScaled(0.000003, 1_000_000)    → "$3"      // per 1M tokens
 *   formatScaled(0.5,        1)          → "$0.5"    // per request
 *   formatScaled(null,       1_000_000)  → "-"
 *   formatScaled(0.000003, 1_000_000, 2) → "$3.00"   // pad to ≥2 decimals
 *
 * Uses toPrecision(10) then strips trailing zeros to avoid IEEE 754 display noise.
 */
export function formatScaled(value: number | null, scale: number, minFractionDigits = 0): string {
  if (value == null) return '-'
  let result = (value * scale).toPrecision(10).replace(/\.?0+$/, '')
  if (minFractionDigits > 0 && !result.includes('e')) {
    const dot = result.indexOf('.')
    const digits = dot === -1 ? 0 : result.length - dot - 1
    if (digits < minFractionDigits) {
      result = (dot === -1 ? `${result}.` : result) + '0'.repeat(minFractionDigits - digits)
    }
  }
  return `$${result}`
}
