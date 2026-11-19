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
import { createStore } from 'zustand/vanilla'

import {
  draftSchema,
  type Draft,
} from '@/features/system-settings/operations/azure-dispatch-rules/lib'

export function createAzureRuleDraftStore(owner: string | number) {
  const key = `azure-dispatch-rules-prototype-v1:${owner}`
  let draft: Draft | null = null
  let loadFailed = false
  try {
    const raw = localStorage.getItem(key)
    if (raw) draft = draftSchema.parse(JSON.parse(raw))
  } catch {
    loadFailed = true
  }
  return createStore<{
    draft: Draft | null
    loadFailed: boolean
    save: (draft: Draft) => void
  }>((set) => ({
    draft,
    loadFailed,
    save: (value) => {
      const parsed = draftSchema.parse(value)
      localStorage.setItem(key, JSON.stringify(parsed))
      set({ draft: parsed, loadFailed: false })
    },
  }))
}
