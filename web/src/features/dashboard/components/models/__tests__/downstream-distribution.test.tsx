import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { DownstreamDistributionCharts } from '../downstream-distribution-charts'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
test('a failed downstream is marked unavailable rather than rendered as zero consumption', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          name: '23000',
          error: 'Downstream statistics sync failed',
          data: [],
          updated_at: 0,
        },
      ],
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <DownstreamDistributionCharts
        filters={{
          start_timestamp: new Date(1000_000),
          end_timestamp: new Date(2000_000),
          time_granularity: 'hour',
        }}
      />
    </QueryClientProvider>
  )
  await waitFor(() =>
    expect(
      screen.getByText('23000: Downstream statistics sync failed')
    ).toBeInTheDocument()
  )
  expect(get.mock.calls.every(([url]) => url === '/api/data/downstreams')).toBe(
    true
  )
  expect(screen.getByRole('region', { name: '23000' })).not.toHaveTextContent(
    'Total:'
  )
  client.clear()
})

test('renders a separate time distribution for each synchronized downstream', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: ['23000', 'noahgrape', 'shangliu'].map((name) => ({
        name,
        error: '',
        updated_at: 2000,
        data: [
          { created_at: 1000, model_name: 'claude', quota: 500000, count: 1 },
        ],
      })),
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <DownstreamDistributionCharts
        filters={{
          start_timestamp: new Date(1000_000),
          end_timestamp: new Date(2000_000),
          time_granularity: 'hour',
        }}
      />
    </QueryClientProvider>
  )
  await waitFor(() =>
    expect(
      screen.getByText('noahgrape · Quota Distribution')
    ).toBeInTheDocument()
  )
  for (const name of ['23000', 'noahgrape', 'shangliu']) {
    expect(screen.getByText(`${name} · Quota Distribution`)).toBeInTheDocument()
  }
  client.clear()
})
