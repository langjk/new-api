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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { toIntlLocale } from '@/i18n/languages'

import type { ModelUsageSite } from '../../api'
import { recentUsageAvailable } from '../../lib/model-totals'

export function ModelTotalsSources(props: {
  sites: ModelUsageSite[]
  now: number
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const timestamp = (value: number | undefined) =>
    value ? new Date(value * 1000).toLocaleString(locale) : '—'
  return (
    <section aria-label={t('Data coverage')} className='space-y-2'>
      <h3 className='font-medium'>{t('Data coverage')}</h3>
      <StaticDataTable
        data={props.sites}
        getRowKey={(site) => site.name}
        tableClassName='min-w-[680px]'
        columns={[
          { id: 'source', header: t('Source'), cell: (site) => site.name },
          {
            id: 'history',
            header: t('Historical data'),
            cell: (site) => {
              const data = site.model_totals
              if (!data?.history_updated_at) return t('Unavailable')
              const stale =
                data.history_error || props.now - data.history_updated_at > 900
              return (
                <div className='space-y-1'>
                  <p>
                    {stale ? t('Last successful sync') : t('Available history')}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {t('Last sync')}: {timestamp(data.history_updated_at)}
                  </p>
                  {data.history_from > 0 && (
                    <p className='text-muted-foreground text-xs'>
                      {t('Earliest record')}: {timestamp(data.history_from)}
                    </p>
                  )}
                </div>
              )
            },
          },
          {
            id: 'recent',
            header: t('Five-minute window'),
            cell: (site) => {
              const data = site.model_totals
              if (!recentUsageAvailable(site, props.now)) {
                return t('Unavailable or stale')
              }
              return (
                <span className='text-xs'>
                  {timestamp(data?.window_start)} –{' '}
                  {timestamp(data?.window_end)}
                </span>
              )
            },
          },
        ]}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'The server syncs periodically. API delays and rate limits can delay updates; windows older than 15 minutes are excluded from recent totals.'
        )}
      </p>
    </section>
  )
}
