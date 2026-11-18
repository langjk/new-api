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
import { ChevronUp } from 'lucide-react'
import { z } from 'zod'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'

// Support both legacy single-object metadata and one result per deployment.
const probeSchema = z.object({
  model: z.string().optional(),
  version: z.string().optional(),
  tier: z.number().int().min(1).max(6).optional(),
  region: z.string().optional(),
  tpm: z.number().optional(),
  rpm: z.number().optional(),
  offer: z.string().optional(),
  err: z.string().optional(),
  status: z.number().int().optional(),
  probed_at: z.number().optional(),
})
type AzureProbeEntry = z.infer<typeof probeSchema>

function parseAzureMeta(meta: string | null | undefined): AzureProbeEntry[] {
  if (!meta) return []
  try {
    const parsed: unknown = JSON.parse(meta)
    const entries: unknown[] = Array.isArray(parsed) ? parsed : [parsed]
    return entries.flatMap((entry) => {
      const result = probeSchema.safeParse(entry)
      return result.success ? [result.data] : []
    })
  } catch {
    return []
  }
}

function isProvisioned(entry: AzureProbeEntry): boolean {
  return entry.offer?.trim().toLowerCase() === 'ptum'
}

export function AzureQuotaBadge(props: { meta: string | null | undefined }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const entries = parseAzureMeta(props.meta)
  const tiers = entries
    .filter((entry) => !isProvisioned(entry))
    .flatMap((entry) => (entry.tier == null ? [] : [entry.tier]))
  const minTier = tiers.length ? Math.min(...tiers) : undefined
  const maxTier = tiers.length ? Math.max(...tiers) : undefined
  const hasHigherTier =
    minTier != null && maxTier != null && maxTier > minTier
  const hasPTU = entries.some(isProvisioned)
  const region = entries.find((entry) => entry.region)?.region

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              tabIndex={0}
              className='flex shrink-0 items-center gap-1.5 text-xs font-medium whitespace-nowrap'
            />
          }
        >
          {minTier != null && (
            <span className='inline-flex items-center text-rose-500'>
              <span>Tier {minTier}</span>
              {hasHigherTier && (
                <ChevronUp
                  aria-label={t('Contains higher Azure tiers')}
                  className='ml-0.5 h-3.5 w-3.5'
                />
              )}
            </span>
          )}
          {hasPTU && (
            <span className='text-sky-600 dark:text-sky-400'>PTU</span>
          )}
          {minTier == null && !hasPTU && (
            <span className='text-muted-foreground'>
              {entries.length ? t('Tier unknown') : t('Tier pending')}
            </span>
          )}
          {region && <span className='text-orange-500'>{region}</span>}
        </TooltipTrigger>
        <TooltipContent side='top'>
          <div className='space-y-1.5 text-xs'>
            <div className='font-medium'>{t('Azure quota details')}</div>
            {minTier != null && (
              <div>
                {t('Azure quota tier: lowest across tested deployments')}
              </div>
            )}
            {entries.length === 0 && (
              <div>{t('Waiting for Azure quota probe.')}</div>
            )}
            {entries.map((entry) => {
              const provisioned = isProvisioned(entry)
              let label = t('Tier unknown')
              let reason = ''
              if (provisioned) {
                label = 'PTU'
                reason = t(
                  'Provisioned throughput (PTU); Global Tier cannot be inferred from this response.'
                )
              } else if (entry.tier != null) {
                label = `Tier ${entry.tier}`
              } else if (
                entry.err ||
                (entry.status != null && entry.status >= 400)
              ) {
                reason = t('Azure quota probe failed; Tier is unavailable.')
              } else if (entry.tpm == null || entry.tpm <= 0) {
                reason = t('The upstream did not return a usable TPM quota.')
              } else {
                reason = t(
                  'No Global Tier mapping is available for this model.'
                )
              }
              // Older dispatcher versions persisted raw transport exception URLs.
              let error = entry.err
              if (error && /^[A-Za-z]+(?:Error|Timeout):/.test(error)) {
                error = t('Azure quota probe failed; Tier is unavailable.')
              }
              return (
                <div key={JSON.stringify(entry)}>
                  <div className='font-medium'>
                    {[entry.model, label].filter(Boolean).join(' · ')}
                  </div>
                  <div>
                    {[
                      entry.version,
                      entry.region,
                      entry.tpm != null
                        ? `TPM ${formatNumber(entry.tpm, locale)}`
                        : undefined,
                      entry.rpm != null
                        ? `RPM ${formatNumber(entry.rpm, locale)}`
                        : undefined,
                      entry.status != null ? `HTTP ${entry.status}` : undefined,
                      entry.probed_at
                        ? formatTimestampToDate(entry.probed_at)
                        : undefined,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </div>
                  {reason && <div>{reason}</div>}
                  {error && <div>{error}</div>}
                </div>
              )
            })}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
