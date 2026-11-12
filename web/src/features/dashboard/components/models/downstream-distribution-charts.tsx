import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { createServerError } from '@/lib/server-error-message'
import { computeTimeRange } from '@/lib/time'

import { getDownstreamUsage } from '../../api'
import { getDefaultDays } from '../../lib'
import type { DashboardFilters } from '../../types'
import { ConsumptionDistributionChart } from './consumption-distribution-chart'

const SITES = ['23000', 'noahgrape', 'shangliu']
export function DownstreamDistributionCharts({
  filters,
}: {
  filters: DashboardFilters
}) {
  const { t } = useTranslation()
  const range = computeTimeRange(
    getDefaultDays(filters.time_granularity),
    filters.start_timestamp,
    filters.end_timestamp
  )
  const query = useQuery({
    queryKey: ['downstream-usage', range.start_timestamp, range.end_timestamp],
    queryFn: async ({ signal }) => {
      const response = await getDownstreamUsage(range, signal)
      if (!response.success) {
        throw createServerError(
          response,
          t('Downstream statistics are not available yet')
        )
      }
      return response.data ?? []
    },
    refetchInterval: 60_000,
    staleTime: 30_000,
    meta: { errorToast: false },
  })
  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Downstream site totals, synchronized on the server every 5 minutes. Last 30 days; root only.'
        )}
      </p>
      {query.isError && (
        <ErrorState
          title={t('Downstream statistics are not available yet')}
          onRetry={() => {
            void query.refetch()
          }}
        />
      )}
      {SITES.map((name) => {
        const site = query.data?.find((item) => item.name === name)
        const unavailable = !query.isPending && !site
        return (
          <section key={name} aria-label={name} className='space-y-2'>
            {(site?.error || unavailable) && (
              <p role='status' className='text-destructive text-sm'>
                {name}: {t('Downstream statistics sync failed')}
              </p>
            )}
            {site?.updated_at ? (
              <p className='text-muted-foreground text-xs'>
                {t('Last sync')}:{' '}
                {new Date(site.updated_at * 1000)
                  .toISOString()
                  .replace('T', ' ')
                  .replace('.000Z', ' UTC')}
              </p>
            ) : null}
            {site && !site.error && (
              <ConsumptionDistributionChart
                title={`${name} · ${t('Quota Distribution')}`}
                data={site.data}
                defaultChartType='bar'
                timeGranularity={filters.time_granularity}
              />
            )}
            {site?.error && site.data.length > 0 && (
              <ConsumptionDistributionChart
                title={`${name} · ${t('Last successful sync')}`}
                data={site.data}
                defaultChartType='bar'
                timeGranularity={filters.time_granularity}
              />
            )}
            {query.isPending && (
              <ConsumptionDistributionChart
                title={`${name} · ${t('Quota Distribution')}`}
                data={[]}
                loading
                defaultChartType='bar'
                timeGranularity={filters.time_granularity}
              />
            )}
          </section>
        )
      })}
    </div>
  )
}
