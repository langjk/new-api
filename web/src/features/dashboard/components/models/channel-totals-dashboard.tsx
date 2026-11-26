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
import { useQuery } from '@tanstack/react-query'
import { Clock3, Coins } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { createServerError } from '@/lib/server-error-message'

import { getChannelUsageTotals, type ChannelUsageGroup } from '../../api'
import { StatCard } from '../ui/stat-card'

function UsageTable(props: {
  rows: ChannelUsageGroup[]
  users?: boolean
  available: boolean
  money: (quota: number) => string
}) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      data={props.rows}
      getRowKey={(row) => (props.users ? row.user_id : row.source)}
      tableProps={{
        'aria-label': t(
          props.users ? 'Consumption by user' : 'Consumption by channel source'
        ),
      }}
      tableClassName='min-w-[640px]'
      emptyContent={<EmptyState title={t('No data available')} />}
      columns={[
        {
          id: 'name',
          header: t(props.users ? 'User' : 'Channel source'),
          cell: (row) => (
            <div className='max-w-xs break-all'>
              <p className='font-medium'>
                {props.users
                  ? row.display_name || row.username || t('Unknown')
                  : row.source}
              </p>
              {props.users && (
                <p className='text-muted-foreground text-xs'>
                  {row.username} · ID {row.user_id}
                </p>
              )}
            </div>
          ),
        },
        {
          id: 'count',
          header: t('Channels'),
          className: 'text-right',
          cellClassName: 'text-right tabular-nums',
          cell: (row) => row.channel_count,
        },
        {
          id: 'quota',
          header: t('Cumulative consumption'),
          className: 'text-right',
          cellClassName: 'text-right whitespace-nowrap tabular-nums',
          cell: (row) => props.money(row.quota),
        },
        {
          id: 'recent',
          header: t('New consumption synced in five minutes'),
          className: 'text-right',
          cellClassName: 'text-right whitespace-nowrap tabular-nums',
          cell: (row) =>
            props.available ? props.money(row.recent_quota) : '—',
        },
      ]}
    />
  )
}

export function ChannelTotalsDashboard() {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<'quota' | 'recent_quota'>('quota')
  const query = useQuery({
    queryKey: ['channel-usage-totals'],
    queryFn: async ({ signal }) => {
      const response = await getChannelUsageTotals(signal)
      if (!response.success || !response.data) {
        throw createServerError(
          response,
          t('Downstream statistics are not available yet')
        )
      }
      return response
    },
    refetchInterval: 30_000,
    staleTime: 15_000,
    meta: { errorToast: false },
  })
  if (query.isPending) return <LoadingState />
  if (query.isError || !query.data.data) {
    return (
      <ErrorState
        title={t('Downstream statistics are not available yet')}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  const data = query.data.data
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const money = (quota: number) =>
    formatBillingCurrencyFromUSD(quota / query.data.quota_per_unit, {
      locale,
      abbreviate: false,
      digitsLarge: 6,
      digitsSmall: 6,
    })
  const stamp = (value: number) =>
    value ? new Date(value * 1000).toLocaleString(locale) : '—'
  const now = Date.now() / 1000
  const available =
    data.metric === 'channel_sync_delta' &&
    !data.error &&
    !data.recent_error &&
    data.window_end - data.window_start >= 240 &&
    data.window_end - data.window_start <= 360 &&
    now >= data.window_end &&
    now - data.window_end <= 900
  const total = data.sources.reduce((sum, row) => sum + row.quota, 0)
  const recent = data.sources.reduce((sum, row) => sum + row.recent_quota, 0)
  const filtered = (rows: ChannelUsageGroup[]) =>
    rows
      .filter((row) =>
        `${row.source} ${row.username} ${row.display_name} ${row.user_id}`
          .toLowerCase()
          .includes(search.trim().toLowerCase())
      )
      .sort(
        (a, b) =>
          b[sort] - a[sort] ||
          a.source.localeCompare(b.source) ||
          a.user_id - b.user_id
      )
  let status = t('Unavailable or stale')
  if (data.recent_error === 'awaiting_baseline') {
    status = t('Awaiting next sample')
  }
  if (data.recent_error === 'counter_changed') {
    status = t('Counter changed; rebuilding baseline')
  }
  if (data.recent_error === 'sampling_gap') {
    status = t('Sampling interval exceeded; rebuilding baseline')
  }
  return (
    <div className='min-w-0 space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Channel counters include all available synchronized history. Recent usage is the increase between samples about five minutes apart, subject to downstream billing delays.'
        )}
      </p>
      <div className='grid gap-3 sm:grid-cols-2'>
        <StatCard
          title={t('Cumulative consumption')}
          icon={Coins}
          value={money(total)}
          description={`${t('Last sync')}: ${stamp(data.updated_at)}`}
        />
        <StatCard
          title={t('New consumption synced in five minutes')}
          icon={Clock3}
          tone='accent-2'
          value={available ? money(recent) : '—'}
          description={
            available
              ? `${stamp(data.window_start)} – ${stamp(data.window_end)}`
              : status
          }
        />
      </div>
      <p role='status' className='text-muted-foreground text-sm'>
        {t(
          data.all_users
            ? 'Administrators see all channel owners; users see only their own channels.'
            : 'Only channels owned by your account are included.'
        )}
      </p>
      {data.error && (
        <p role='alert'>
          {t('Last successful sync')}: {stamp(data.updated_at)}
        </p>
      )}
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          aria-label={t('Search sources or users')}
          placeholder={t('Search sources or users')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className='w-full sm:max-w-xs'
        />
        <Button
          variant='outline'
          aria-pressed={sort === 'quota'}
          onClick={() => setSort('quota')}
        >
          {t('Sort by cumulative consumption')}
        </Button>
        <Button
          variant='outline'
          aria-pressed={sort === 'recent_quota'}
          onClick={() => setSort('recent_quota')}
        >
          {t('Sort by synced increase')}
        </Button>
        <Button
          variant='ghost'
          disabled={query.isFetching}
          onClick={() => {
            void query.refetch()
          }}
        >
          {t('Refresh')}
        </Button>
      </div>
      <section className='space-y-2'>
        <h3 className='font-medium'>{t('Consumption by channel source')}</h3>
        <UsageTable
          rows={filtered(data.sources)}
          available={available}
          money={money}
        />
      </section>
      <section className='space-y-2'>
        <h3 className='font-medium'>{t('Consumption by user')}</h3>
        <UsageTable
          rows={filtered(data.users)}
          users
          available={available}
          money={money}
        />
      </section>
      <p className='text-muted-foreground text-xs'>
        {t(
          'User consumption belongs to channels they created. Deleted channels observed after this dashboard starts retain their last cumulative amount; earlier deleted history cannot be reconstructed.'
        )}
      </p>
    </div>
  )
}
