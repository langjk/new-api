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
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'

import type { ChannelUsageGroup } from '../../api'

export function UserDownstreamDialog(props: {
  user?: ChannelUsageGroup
  onClose: () => void
  money: (value: number) => string
  recentAvailable: boolean
  period: boolean
  periodAvailable: boolean
}) {
  const { t } = useTranslation()
  const rows = [...(props.user?.downstreams ?? [])]
    .filter(
      (row) =>
        row.downstream !== '__unattributed__' ||
        row.quota ||
        row.recent_quota ||
        row.period_quota
    )
    .sort(
      (a, b) =>
        (props.period ? b.period_quota - a.period_quota : b.quota - a.quota) ||
        a.downstream.localeCompare(b.downstream)
    )
  let total = props.money(props.user?.quota ?? 0)
  if (props.period) {
    total = props.periodAvailable
      ? props.money(props.user?.period_quota ?? 0)
      : '—'
  }
  return (
    <Dialog
      open={Boolean(props.user)}
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Downstream consumption')}
      description={`${props.user?.display_name || props.user?.username || ''} · ID ${props.user?.user_id ?? ''}`}
      contentClassName='sm:max-w-3xl'
      bodyClassName='space-y-4'
    >
      <StaticDataTable
        data={rows}
        getRowKey={(row) => row.downstream}
        tableProps={{ 'aria-label': t('Downstream consumption') }}
        tableClassName='min-w-[560px]'
        emptyContent={<EmptyState title={t('No data available')} />}
        columns={[
          {
            id: 'name',
            header: t('Downstream'),
            cell: (row) => (
              <span className='break-all'>
                {row.downstream === '__unattributed__'
                  ? t('Unattributed / synchronization difference')
                  : row.downstream}
              </span>
            ),
          },
          {
            id: 'quota',
            header: t('Cumulative consumption'),
            cellClassName: 'text-right whitespace-nowrap tabular-nums',
            cell: (row) => props.money(row.quota),
          },
          {
            id: 'recent',
            header: t('New consumption synced in five minutes'),
            cellClassName: 'text-right whitespace-nowrap tabular-nums',
            cell: (row) =>
              props.recentAvailable ? props.money(row.recent_quota) : '—',
          },
          ...(props.period
            ? [
                {
                  id: 'period',
                  header: t('Consumption in selected period'),
                  cellClassName: 'text-right whitespace-nowrap tabular-nums',
                  cell: (row: (typeof rows)[number]) =>
                    props.periodAvailable ? props.money(row.period_quota) : '—',
                },
              ]
            : []),
        ]}
      />
      <p className='font-medium'>
        {t('Total')}: {total}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Includes current and archived downstream channels. Unattributed amounts and signed synchronization differences are shown separately so the breakdown matches the user total.'
        )}
      </p>
      {props.period && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Period totals follow the same sampled range and coverage shown on the dashboard.'
          )}
        </p>
      )}
    </Dialog>
  )
}
