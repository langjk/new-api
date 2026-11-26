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
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { ModelUsageSite } from '../../../api'
import { aggregateModelTotals } from '../../../lib/model-totals'
import {
  DASHBOARD_SECTION_IDS,
  getDashboardSectionNavItems,
} from '../../../section-registry'
import { ModelTotalsDashboard } from '../model-totals-dashboard'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function source(
  name: string,
  overrides: Partial<NonNullable<ModelUsageSite['model_totals']>> = {}
): ModelUsageSite {
  const now = Math.floor(Date.now() / 1000)
  return {
    name,
    model_totals: {
      history_models: [
        { model_name: 'alpha', quota: 1000000 },
        { model_name: 'beta', quota: 500000 },
      ],
      history_unattributed: 0,
      history_from: 1000,
      history_updated_at: now,
      history_error: '',
      recent_models: [{ model_name: 'beta', quota: 100000 }],
      recent_unattributed: 0,
      window_start: now - 300,
      window_end: now,
      recent_updated_at: now,
      recent_error: '',
      ...overrides,
    },
  }
}

function show(sites: ModelUsageSite[]) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: sites, quota_per_unit: 500000 },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ModelTotalsDashboard />
    </QueryClientProvider>
  )
  return client
}

test('same models across sources add once and stale recent windows never appear as current', () => {
  const now = Date.now() / 1000
  const data = aggregateModelTotals(
    [
      source('one'),
      source('two', { window_start: now - 1300, window_end: now - 1000 }),
      { name: 'missing', model_totals: null },
    ],
    now
  )
  expect(data.historyTotal).toBe(3000000)
  expect(data.recentTotal).toBe(100000)
  expect(data.recentSources).toBe(1)
  expect(data.rows.find((row) => row.model === 'alpha')?.total).toBe(2000000)
})

test('unattributed totals are included in summary without inventing a model', () => {
  const data = aggregateModelTotals(
    [
      source('one', {
        history_models: [],
        history_unattributed: 123,
        recent_models: [],
        recent_unattributed: 12,
      }),
    ],
    Date.now() / 1000
  )
  expect(data.historyTotal).toBe(123)
  expect(data.recentTotal).toBe(12)
  expect(data.rows).toEqual([])
})

test('search and keyboard sort update visible model rows while retaining table scrolling', async () => {
  const user = userEvent.setup()
  const client = show([source('one')])
  const table = await screen.findByRole('table', {
    name: 'Model consumption totals',
  })
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('alpha')
  const sort = screen.getByRole('button', {
    name: 'Sort by five-minute consumption',
  })
  sort.focus()
  await user.keyboard('{Enter}')
  expect(sort).toHaveAttribute('aria-pressed', 'true')
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('beta')
  expect(table).toHaveClass('min-w-[480px]')
  await user.type(
    screen.getByRole('textbox', { name: 'Search models' }),
    'alpha'
  )
  expect(within(table).queryByText('beta')).not.toBeInTheDocument()
  await user.clear(screen.getByRole('textbox', { name: 'Search models' }))
  expect(within(table).getByText('beta')).toBeInTheDocument()
  client.clear()
})

test('missing sources and failed recent sync are visibly unavailable rather than zero', async () => {
  const client = show([
    source('failed', { recent_error: 'Five-minute model totals unavailable' }),
    { name: 'missing', model_totals: null },
  ])
  const table = await screen.findByRole('table', {
    name: 'Model consumption totals',
  })
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('—')
  expect(screen.getByText('Unavailable')).toBeInTheDocument()
  expect(screen.getAllByText('Unavailable or stale')).toHaveLength(2)
  client.clear()
})

test('empty model data exposes an empty state and a long model remains readable', async () => {
  const client = show([
    source('one', {
      history_models: [{ model_name: 'model-'.repeat(40), quota: 1 }],
    }),
  ])
  const table = await screen.findByRole('table', {
    name: 'Model consumption totals',
  })
  expect(within(table).getByText('model-'.repeat(40))).toHaveClass('break-all')
  await userEvent.type(
    screen.getByRole('textbox', { name: 'Search models' }),
    'not-found'
  )
  expect(screen.getByText('No model consumption available')).toBeInTheDocument()
  client.clear()
})

test('API failure renders retry action instead of consumption totals', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: false, message: 'unavailable' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ModelTotalsDashboard />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText('Downstream statistics are not available yet')
  ).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  client.clear()
})

test('totals navigation is available to the same users as downstream charts', () => {
  expect(DASHBOARD_SECTION_IDS).toContain('totals')
  const nav = getDashboardSectionNavItems(((key: string) => key) as never, {
    isAdmin: false,
  })
  expect(JSON.stringify(nav)).toContain('/dashboard/totals')
})

test('pending statistics show loading until the response arrives', async () => {
  let resolve!: (value: unknown) => void
  vi.spyOn(api, 'get').mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done
      }) as never
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ModelTotalsDashboard />
    </QueryClientProvider>
  )
  expect(screen.getByText('Loading...')).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  await act(async () => {
    resolve({ data: { success: true, data: [] } })
  })
  expect(
    await screen.findByText('No model consumption available')
  ).toBeInTheDocument()
  client.clear()
})
