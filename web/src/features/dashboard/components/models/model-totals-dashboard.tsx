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

import { getModelUsageTotals } from '../../api'
import { aggregateModelTotals } from '../../lib/model-totals'
import { StatCard } from '../ui/stat-card'
import { ModelTotalsSources } from './model-totals-sources'

export function ModelTotalsDashboard() {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<'total' | 'recent'>('total')
  const query = useQuery({
    queryKey: ['model-usage-totals'],
    queryFn: async ({ signal }) => {
      const response = await getModelUsageTotals(signal)
      if (!response.success) {
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
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const sites = query.data?.data ?? []
  const now = Date.now() / 1000
  const data = aggregateModelTotals(sites, now)
  const quotaPerUnit = query.data?.quota_per_unit || 500000
  const money = (quota: number) =>
    formatBillingCurrencyFromUSD(quota / quotaPerUnit, {
      abbreviate: false,
      digitsLarge: 2,
      digitsSmall: 6,
      locale,
    })
  const rows = data.rows
    .filter((row) =>
      row.model.toLowerCase().includes(search.trim().toLowerCase())
    )
    .sort((a, b) => b[sort] - a[sort] || a.model.localeCompare(b.model))
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Downstream statistics are not available yet')}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }

  return (
    <div className='min-w-0 space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'All accessible downstream history. Five-minute usage is billed consumption in each source’s latest synchronized window, not the change between syncs.'
        )}
      </p>
      <div className='grid gap-3 sm:grid-cols-2'>
        <StatCard
          title={t('Cumulative consumption')}
          icon={Coins}
          value={data.historySources ? money(data.historyTotal) : '—'}
          description={t('Available sources: {{available}} / {{total}}', {
            available: data.historySources,
            total: sites.length,
          })}
        />
        <StatCard
          title={t('Last five-minute consumption')}
          icon={Clock3}
          tone='accent-2'
          value={data.recentSources ? money(data.recentTotal) : '—'}
          description={t('Current sources: {{available}} / {{total}}', {
            available: data.recentSources,
            total: sites.length,
          })}
        />
      </div>
      <p role='status' className='text-muted-foreground text-sm'>
        {t(
          'Totals include available sources only. Unavailable sources are not zero. Model rows exclude usage without a model breakdown.'
        )}
      </p>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          aria-label={t('Search models')}
          placeholder={t('Search models')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className='w-full sm:max-w-xs'
        />
        <Button
          variant='outline'
          aria-pressed={sort === 'total'}
          onClick={() => setSort('total')}
        >
          {t('Sort by cumulative consumption')}
        </Button>
        <Button
          variant='outline'
          aria-pressed={sort === 'recent'}
          onClick={() => setSort('recent')}
        >
          {t('Sort by five-minute consumption')}
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
      <StaticDataTable
        data={rows}
        getRowKey={(row) => row.model}
        tableProps={{ 'aria-label': t('Model consumption totals') }}
        tableClassName='min-w-[480px]'
        emptyContent={
          <EmptyState title={t('No model consumption available')} />
        }
        columns={[
          {
            id: 'model',
            header: t('Model'),
            cell: (row) => (
              <span className='font-medium break-all'>{row.model}</span>
            ),
          },
          {
            id: 'total',
            header: t('Cumulative consumption'),
            className: 'text-right',
            cellClassName: 'text-right tabular-nums whitespace-nowrap',
            cell: (row) => (data.historySources ? money(row.total) : '—'),
          },
          {
            id: 'recent',
            header: t('Last five-minute consumption'),
            className: 'text-right',
            cellClassName: 'text-right tabular-nums whitespace-nowrap',
            cell: (row) => (data.recentSources ? money(row.recent) : '—'),
          },
        ]}
      />
      {(data.historyUnattributed > 0 || data.recentUnattributed > 0) && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Usage without model breakdown: cumulative {{total}}; five-minute {{recent}}.',
            {
              total: money(data.historyUnattributed),
              recent: money(data.recentUnattributed),
            }
          )}
        </p>
      )}
      <ModelTotalsSources sites={sites} now={now} />
    </div>
  )
}
