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
import { useQueryClient } from '@tanstack/react-query'
import { RotateCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import { repushChannelDispatch } from '../api'
import { channelsQueryKeys } from '../lib'
import {
  dispatchBadgeVariant,
  formatDispatchLabel,
  isRepushable,
  parseDispatchInfo,
  type DispatchEntry,
} from '../lib/dispatch-info'

export function DispatchInfoCell(props: { channelId: number; raw?: string | null }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [target, setTarget] = useState<DispatchEntry | null>(null)
  const [submitting, setSubmitting] = useState(false)

  if (!props.raw) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }
  const entries = parseDispatchInfo(props.raw)
  if (!entries) {
    return <span className='text-xs'>{props.raw}</span>
  }

  const handleConfirm = async () => {
    if (!target?.ds) return
    setSubmitting(true)
    try {
      const res = await repushChannelDispatch(
        props.channelId,
        target.ds,
        target.g ?? ''
      )
      if (!res.success) {
        throw createServerError(res, t('Failed to request re-push'))
      }
      toast.success(t('Re-push requested, the dispatcher will process it shortly'))
      setTarget(null)
      void queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    } catch (error) {
      handleServerError(error, t('Failed to request re-push'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <div className='flex flex-wrap gap-1'>
        {entries.map((e) => {
          const st = String(e.st ?? '')
          const key = `${e.ds ?? ''}/${e.g ?? ''}/${e.rid ?? ''}/${st}`
          const hasDetail =
            e.rid != null || !!e.rsn || e.t != null || !!e.err
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
              {isRepushable(e) && (
                <Button
                  variant='ghost'
                  size='icon-xs'
                  aria-label={t('Re-push')}
                  title={t('Re-push')}
                  onClick={() => setTarget(e)}
                >
                  <RotateCw aria-hidden='true' />
                </Button>
              )}
            </span>
          )
        })}
      </div>
      <ConfirmDialog
        open={target != null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('Confirm Re-push')}
        desc={t(
          'Re-push this channel to {{target}}? A new downstream channel will be created with the same key; the old one is kept. A disabled channel is re-enabled.',
          { target: target ? `${target.ds}/${target.g ?? ''}` : '' }
        )}
        confirmText={t('Re-push')}
        isLoading={submitting}
        handleConfirm={() => void handleConfirm()}
      />
    </>
  )
}
