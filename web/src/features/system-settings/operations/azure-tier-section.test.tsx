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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { azureTierTableSchema } from './azure-tier-api'
import { AzureTierSection } from './azure-tier-section'

const table = {
  revision: 'original',
  rows: [
    {
      model: 'gpt-6.2-astra',
      tpm: [250000, 1000000, 2500000, 7000000, 10000000, 15000000],
    },
  ],
}
async function renderTable() {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: table },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <AzureTierSection />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByDisplayValue('gpt-6.2-astra')
}

describe('Azure Tier editor', () => {
  it('adds a model and saves all rows with the original revision', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({
        data: { success: true, data: { ...table, revision: 'saved' } },
      })
    await renderTable()
    await userEvent.click(screen.getByRole('button', { name: 'Add model' }))
    await userEvent.type(screen.getByLabelText('Model 1'), 'gpt-next')
    const threshold = screen.getByLabelText('Model 1 Tier 1 TPM')
    await userEvent.clear(threshold)
    await userEvent.type(threshold, '250000')
    await userEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/azure-tiers/', {
        revision: 'original',
        rows: [
          { model: 'gpt-next', tpm: [250000, 0, 0, 0, 0, 0] },
          ...table.rows,
        ],
      })
    )
  })

  it('keeps unsaved edits when the server rejects a stale revision', async () => {
    vi.spyOn(api, 'put').mockRejectedValue(new Error('Conflict'))
    await renderTable()
    const name = screen.getByLabelText('Model 1')
    await userEvent.clear(name)
    await userEvent.type(name, 'edited')
    await userEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save Settings' })
      ).toBeEnabled()
    )
    expect(name).toHaveValue('edited')
  })

  it('validates duplicate models and descending thresholds, allows unpublished gaps', () => {
    expect(
      azureTierTableSchema.safeParse({
        ...table,
        rows: [...table.rows, ...table.rows],
      }).success
    ).toBe(false)
    expect(
      azureTierTableSchema.safeParse({
        ...table,
        rows: [{ model: 'x', tpm: [20, 10, 0, 0, 0, 0] }],
      }).success
    ).toBe(false)
    expect(
      azureTierTableSchema.safeParse({
        ...table,
        rows: [{ model: 'x', tpm: [10, 0, 20, 20, 0, 30] }],
      }).success
    ).toBe(true)
  })
})
