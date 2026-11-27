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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { ChannelUsageTotals } from '../../../api'
import { DEFAULT_DASHBOARD_CHART_PREFERENCES } from '../../../constants'
import { ChannelTotalsDashboard } from '../channel-totals-dashboard'
import { ModelsFilter } from '../models-filter-dialog'

const originalAnimations = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'getAnimations'
)
beforeEach(() => {
  Object.defineProperty(Element.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})
afterEach(() => {
  if (originalAnimations) {
    Object.defineProperty(
      Element.prototype,
      'getAnimations',
      originalAnimations
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
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
  expect(
    screen.getByRole('button', {
      name: `View downstream consumption: ${'Owner'.repeat(40)}`,
    })
  ).toHaveClass('whitespace-normal')
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

test('clicking a user opens downstream amounts and keyboard Escape closes details', async () => {
  const data = fixture()
  data.users[0].downstreams = [
    {
      downstream: 'noahgrape',
      quota: 1000000,
      recent_quota: 1000,
      period_quota: 200000,
    },
    {
      downstream: 'man',
      quota: 600000,
      recent_quota: 2000,
      period_quota: 100000,
    },
    {
      downstream: '__unattributed__',
      quota: -100000,
      recent_quota: 0,
      period_quota: 0,
    },
  ]
  const client = show(data)
  const user = userEvent.setup()
  const button = await screen.findByRole('button', {
    name: 'View downstream consumption: Alice',
  })
  button.focus()
  await user.keyboard('{Enter}')
  const dialog = await screen.findByRole('dialog', {
    name: 'Downstream consumption',
  })
  const table = within(dialog).getByRole('table', {
    name: 'Downstream consumption',
  })
  expect(within(table).getByText('noahgrape')).toBeInTheDocument()
  expect(within(table).getByText('man')).toBeInTheDocument()
  expect(
    within(table).getByText('Unattributed / synchronization difference')
  ).toBeInTheDocument()
  expect(table).toHaveClass('min-w-[560px]')
  expect(within(table).getByText('-$0.2')).toBeInTheDocument()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  client.clear()
})

test('time filter applies a seven-day query and all time removes the query', async () => {
  const client = show()
  const user = userEvent.setup()
  await screen.findByRole('table', { name: 'Consumption by user' })
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Consumption time range',
  })
  expect(within(dialog).queryByText('Time Granularity')).not.toBeInTheDocument()
  expect(within(dialog).queryByText('Username')).not.toBeInTheDocument()
  expect(
    within(dialog).getByRole('button', { name: 'Apply Filters' })
  ).toBeDisabled()
  await user.click(within(dialog).getByRole('button', { name: '7 Days' }))
  await user.click(
    within(dialog).getByRole('button', { name: 'Apply Filters' })
  )
  await screen.findByRole('table', { name: 'Consumption by user' })
  const calls = vi.mocked(api.get).mock.calls
  const params = calls.at(-1)?.[1]?.params
  expect(params.end_timestamp - params.start_timestamp).toBe(7 * 86400)
  await user.click(screen.getByRole('button', { name: 'All time' }))
  expect(screen.getByRole('button', { name: 'All time' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  client.clear()
})

test('unavailable historical range displays a gap warning instead of zero consumption', async () => {
  const data = fixture({
    range: {
      active: true,
      requested_start: 100,
      requested_end: 1000,
      actual_start: 0,
      actual_end: 0,
      available_from: 2000,
      available_until: 2300,
      samples: 0,
      status: 'unavailable',
    },
  })
  const client = show(data)
  expect(
    await screen.findByText(
      'No recorded samples are available for this period.'
    )
  ).toBeInTheDocument()
  const table = screen.getByRole('table', { name: 'Consumption by user' })
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('—')
  client.clear()
})

test('partial history is labeled and selected period amounts also appear in user detail', async () => {
  const data = fixture({
    range: {
      active: true,
      requested_start: 100,
      requested_end: 2000,
      actual_start: 1000,
      actual_end: 1300,
      available_from: 1000,
      available_until: 1300,
      samples: 1,
      status: 'partial',
    },
  })
  data.users[0].period_quota = 200000
  data.users[0].downstreams = [
    {
      downstream: 'noahgrape',
      quota: 1500000,
      recent_quota: 3000,
      period_quota: 200000,
    },
  ]
  const client = show(data)
  const user = userEvent.setup()
  expect(
    await screen.findByText(
      'Partial history: the displayed amount covers only available samples.'
    )
  ).toBeInTheDocument()
  await user.click(
    screen.getByRole('button', { name: 'View downstream consumption: Alice' })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Downstream consumption',
  })
  expect(
    within(dialog).getByText('Consumption in selected period')
  ).toBeInTheDocument()
  expect(within(dialog).getByText('$0.4')).toBeInTheDocument()
  client.clear()
})

test('time-only filter rejects inverted dates while existing model filters keep chart controls', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  const filters = {
    start_timestamp: new Date(2026, 10, 28),
    end_timestamp: new Date(2026, 10, 27),
    time_granularity: 'hour' as const,
    username: '',
  }
  const view = render(
    <ModelsFilter
      preferences={DEFAULT_DASHBOARD_CHART_PREFERENCES}
      currentFilters={filters}
      onFilterChange={onChange}
      onReset={() => {}}
      timeOnly
    />
  )
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  expect(screen.getByRole('button', { name: 'Apply Filters' })).toBeDisabled()
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Select a valid time range of up to 30 days'
  )
  expect(onChange).not.toHaveBeenCalled()
  view.unmount()
  render(
    <ModelsFilter
      preferences={DEFAULT_DASHBOARD_CHART_PREFERENCES}
      currentFilters={filters}
      onFilterChange={onChange}
      onReset={() => {}}
    />
  )
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  expect(screen.getByText('Time Granularity')).toBeInTheDocument()
})
