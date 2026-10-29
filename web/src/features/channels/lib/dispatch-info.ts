/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/**
 * One dispatch target written by the external dispatcher into dispatch_info:
 * ds=downstream, g=downstream group, st=status, rid=downstream channel id,
 * usd=used quota in USD, rsn/t=disable reason/time, err=delivery error,
 * old=archived downstream channel replaced by a re-push
 */
export type DispatchEntry = {
  ds?: string
  g?: string
  st?: string
  rid?: number
  usd?: number
  rsn?: string
  t?: number
  err?: string
  old?: number
}

const REPUSHABLE_STATUSES = new Set(['ok', 'dead', 'failed'])

export function parseDispatchInfo(raw: string): DispatchEntry[] | null {
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as DispatchEntry[]) : null
  } catch {
    return null
  }
}

/** Badge label: downstream/group #id:status $usd */
export function formatDispatchLabel(entry: DispatchEntry): string {
  const st = String(entry.st ?? '')
  if (!entry.ds) return st
  let label = entry.ds
  if (entry.g) label += `/${entry.g}`
  if (entry.rid != null) label += ` #${entry.rid}`
  label += `:${st}`
  if (entry.usd != null) label += ` $${entry.usd}`
  return label
}

/** Only current (not archived) ok/dead/failed targets can be re-pushed */
export function isRepushable(entry: DispatchEntry): boolean {
  return (
    !entry.old && !!entry.ds && REPUSHABLE_STATUSES.has(String(entry.st ?? ''))
  )
}

export function dispatchBadgeVariant(
  st: string
): 'success' | 'danger' | 'neutral' {
  if (st.includes('ok')) return 'success'
  if (st === 'dead' || st.includes('failed')) return 'danger'
  return 'neutral'
}
