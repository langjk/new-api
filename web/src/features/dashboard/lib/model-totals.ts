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
import type { ModelUsageSite } from '../api'

export function recentUsageAvailable(
  site: ModelUsageSite,
  now: number
): boolean {
  const data = site.model_totals
  return Boolean(
    data &&
    data.metric === 'sync_delta' &&
    !data.recent_error &&
    data.recent_updated_at > 0 &&
    data.window_end - data.window_start >= 240 &&
    data.window_end - data.window_start <= 360 &&
    now >= data.window_end &&
    now - data.window_end <= 900
  )
}

export function aggregateModelTotals(sites: ModelUsageSite[], now: number) {
  const models = new Map<
    string,
    { model: string; total: number; recent: number }
  >()
  let historyTotal = 0
  let recentTotal = 0
  let historySources = 0
  let recentSources = 0
  let historyUnattributed = 0
  let recentUnattributed = 0
  for (const site of sites) {
    const data = site.model_totals
    if (!data) continue
    if (data.history_updated_at > 0) {
      historySources++
      historyUnattributed += data.history_unattributed
      historyTotal += data.history_unattributed
      for (const item of data.history_models ?? []) {
        const row = models.get(item.model_name) ?? {
          model: item.model_name,
          total: 0,
          recent: 0,
        }
        row.total += item.quota
        historyTotal += item.quota
        models.set(row.model, row)
      }
    }
    if (recentUsageAvailable(site, now)) {
      recentSources++
      recentUnattributed += data.recent_unattributed
      recentTotal += data.recent_unattributed
      for (const item of data.recent_models ?? []) {
        const row = models.get(item.model_name) ?? {
          model: item.model_name,
          total: 0,
          recent: 0,
        }
        row.recent += item.quota
        recentTotal += item.quota
        models.set(row.model, row)
      }
    }
  }
  return {
    rows: [...models.values()],
    historyTotal,
    recentTotal,
    historySources,
    recentSources,
    historyUnattributed,
    recentUnattributed,
  }
}
