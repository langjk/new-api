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

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'

import {
  dispatchBadgeVariant,
  formatDispatchLabel,
  parseDispatchInfo,
} from '../lib/dispatch-info'

export function DispatchInfoCell(props: {
  channelId: number
  raw?: string | null
}) {
  const { t } = useTranslation()
  if (!props.raw) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }
  const entries = parseDispatchInfo(props.raw)
  if (!entries) {
    return <span className='text-xs'>{props.raw}</span>
  }

  return (
    <div className='flex flex-wrap gap-1'>
      {entries.map((e) => {
        const st = String(e.st ?? '')
        const key = `${e.ds ?? ''}/${e.g ?? ''}/${e.rid ?? ''}/${st}`
        const hasDetail = e.rid != null || !!e.rsn || e.t != null || !!e.err
        const badge = (
          <StatusBadge
            label={formatDispatchLabel(e)}
            variant={dispatchBadgeVariant(st)}
            size='sm'
            copyable={false}
            className={hasDetail ? 'cursor-help' : undefined}
          />
        )
        return (
          <span key={key} className='inline-flex items-center gap-0.5'>
            {hasDetail ? (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger render={badge} />
                  <TooltipContent side='top'>
                    <div className='max-w-xs space-y-0.5 text-xs break-words'>
                      {e.g && (
                        <div>
                          {t('Group')}: {e.g}
                        </div>
                      )}
                      {e.rid != null && <div>ID: {e.rid}</div>}
                      {e.t != null && (
                        <div>
                          {t('Disabled at')}: {formatTimestampToDate(e.t)}
                        </div>
                      )}
                      {e.rsn && (
                        <div>
                          {t('Reason')}: {e.rsn}
                        </div>
                      )}
                      {e.err && (
                        <div>
                          {t('Error')}: {e.err}
                        </div>
                      )}
                    </div>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ) : (
              badge
            )}
          </span>
        )
      })}
    </div>
  )
}
