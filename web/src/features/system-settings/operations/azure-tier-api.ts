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
import { z } from 'zod'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export const azureTierTableSchema = z
  .object({
    revision: z.string(),
    rows: z
      .array(
        z.object({
          model: z
            .string()
            .trim()
            .min(1, 'Required')
            .max(200)
            .regex(/^[^\s,]+$/, 'Invalid model name'),
          tpm: z.array(z.number().int().min(0).max(1000000000000)).length(6),
        })
      )
      .max(500),
  })
  .superRefine((value, ctx) => {
    const seen = new Set<string>()
    value.rows.forEach((row, index) => {
      const model = row.model.toLowerCase()
      if (seen.has(model)) {
        ctx.addIssue({
          code: 'custom',
          path: ['rows', index, 'model'],
          message: 'Duplicate model name',
        })
      }
      seen.add(model)
      let previous = 0
      row.tpm.forEach((amount, tier) => {
        if (amount > 0 && amount < previous) {
          ctx.addIssue({
            code: 'custom',
            path: ['rows', index, 'tpm', tier],
            message: 'TPM thresholds must not decrease',
          })
        }
        if (amount > 0) previous = amount
      })
      if (previous === 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['rows', index, 'model'],
          message: 'Enter at least one TPM threshold',
        })
      }
    })
  })

export type AzureTierTable = z.infer<typeof azureTierTableSchema>
type Response = { success: boolean; message: string; data: AzureTierTable }
export async function getAzureTierTable(): Promise<AzureTierTable> {
  const response = await api.get<Response>('/api/azure-tiers/')
  return requireServerSuccess(response.data).data
}
export async function saveAzureTierTable(
  table: AzureTierTable
): Promise<AzureTierTable> {
  const response = await api.put<Response>('/api/azure-tiers/', table)
  return requireServerSuccess(response.data).data
}
