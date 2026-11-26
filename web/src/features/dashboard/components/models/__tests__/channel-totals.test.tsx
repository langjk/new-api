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

import type { ChannelUsageTotals } from '../../../api'
import { ChannelTotalsDashboard } from '../channel-totals-dashboard'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
function fixture(
  overrides: Partial<ChannelUsageTotals> = {}
): ChannelUsageTotals {
  const now = Math.floor(Date.now() / 1000)
  const common = {
    username: '',
    display_name: '',
    user_id: 0,
    channel_count: 2,
    retained_count: 0,
  }
  return {
    metric: 'channel_sync_delta',
    updated_at: now,
    window_start: now - 300,
    window_end: now,
    error: '',
    recent_error: '',
    all_users: true,
    sources: [
      { ...common, source: 'gpt-openai', quota: 1000000, recent_quota: 1000 },
      { ...common, source: 'claude-azure', quota: 500000, recent_quota: 2000 },
    ],
    users: [
      {
        ...common,
        user_id: 1,
        username: 'alice',
        display_name: 'Alice',
        source: '',
        quota: 1500000,
        recent_quota: 3000,
      },
      {
        ...common,
        user_id: 2,
        username: 'zero',
        display_name: '',
        source: '',
        quota: 0,
        recent_quota: 0,
      },
    ],
    ...overrides,
  }
}
function show(data = fixture()) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data, quota_per_unit: 500000 },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelTotalsDashboard />
    </QueryClientProvider>
  )
  return client
}
test('source and user tables display cumulative and new usage including zero-use users', async () => {
  const client = show()
  const source = await screen.findByRole('table', {
    name: 'Consumption by channel source',
  })
  const users = screen.getByRole('table', { name: 'Consumption by user' })
  expect(within(source).getByText('gpt-openai')).toBeInTheDocument()
  expect(within(users).getByText('Alice')).toBeInTheDocument()
  expect(within(users).getByText('zero')).toBeInTheDocument()
  expect(source).toHaveClass('min-w-[640px]')
  client.clear()
})
test('search and keyboard sorting change both visible tables', async () => {
  const client = show()
  const user = userEvent.setup()
  const table = await screen.findByRole('table', {
    name: 'Consumption by channel source',
  })
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('gpt-openai')
  const sort = screen.getByRole('button', { name: 'Sort by synced increase' })
  sort.focus()
  await user.keyboard('{Enter}')
  expect(sort).toHaveAttribute('aria-pressed', 'true')
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('claude-azure')
  await user.type(
    screen.getByRole('textbox', { name: 'Search sources or users' }),
    'Alice'
  )
  expect(within(table).queryByText('gpt-openai')).not.toBeInTheDocument()
  expect(screen.getByText('Alice')).toBeInTheDocument()
  client.clear()
})
test('first sample shows baseline state and does not present cumulative usage as recent', async () => {
  const client = show(
    fixture({
      recent_error: 'awaiting_baseline',
      window_start: 0,
      window_end: 0,
    })
  )
  expect(await screen.findByText('Awaiting next sample')).toBeInTheDocument()
  const table = screen.getByRole('table', {
    name: 'Consumption by channel source',
  })
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('—')
  client.clear()
})
test('ordinary users see the own-account scope notice and long names wrap', async () => {
  const data = fixture({ all_users: false })
  data.users[0].display_name = 'Owner'.repeat(40)
  const client = show(data)
  expect(
    await screen.findByText('Only channels owned by your account are included.')
  ).toBeInTheDocument()
  expect(screen.getByText('Owner'.repeat(40)).parentElement).toHaveClass(
    'break-all'
  )
  client.clear()
})
test('stale windows keep history but suppress recent values', async () => {
  const now = Math.floor(Date.now() / 1000)
  const client = show(
    fixture({ window_start: now - 1500, window_end: now - 1200 })
  )
  expect(await screen.findByText('Unavailable or stale')).toBeInTheDocument()
  client.clear()
})
test('failed request provides error and retry without zero usage', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelTotalsDashboard />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText('Downstream statistics are not available yet')
  ).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  client.clear()
})
test('pending request displays loading and empty response displays both empty tables', async () => {
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
      <ChannelTotalsDashboard />
    </QueryClientProvider>
  )
  expect(screen.getByText('Loading...')).toBeInTheDocument()
  await act(async () => {
    resolve({
      data: {
        success: true,
        data: fixture({ sources: [], users: [] }),
        quota_per_unit: 500000,
      },
    })
  })
  expect(await screen.findAllByText('No data available')).toHaveLength(2)
  client.clear()
})
