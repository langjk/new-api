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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import { evaluateDraft, type Draft, type SampleChannel } from './lib'

export function RulesPreview(props: { draft: Draft }) {
  const { t } = useTranslation()
  const [sample, setSample] = useState<SampleChannel>({
    tier: 4,
    region: 'swedencentral',
  })
  const result = evaluateDraft(props.draft, sample)
  const statuses = {
    matched: t('Rule matched'),
    unmatched: t('Rule did not match'),
    skipped: t('Skipped after first match'),
    disabled: t('Disabled'),
    incomplete: t('Incomplete rule'),
    unknown: t('Missing sample data'),
  }
  return (
    <Card className='min-w-0 xl:sticky xl:top-4'>
      <CardHeader>
        <CardTitle>{t('Try a simulated channel')}</CardTitle>
        <CardDescription>
          {t(
            'Results update as you edit. No real channel is tested or dispatched.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-5'>
        <FieldGroup className='gap-3'>
          <Field>
            <FieldLabel htmlFor='sample-tier'>
              {t('Sample channel Tier')}
            </FieldLabel>
            <NativeSelect
              id='sample-tier'
              className='w-full'
              value={sample.tier ?? ''}
              onChange={(event) =>
                setSample({
                  ...sample,
                  tier: event.target.value ? Number(event.target.value) : null,
                })
              }
            >
              <NativeSelectOption value=''>{t('Unknown')}</NativeSelectOption>
              {[1, 2, 3, 4, 5, 6].map((tier) => (
                <NativeSelectOption key={tier} value={tier}>
                  Tier {tier}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </Field>
          <Field>
            <FieldLabel htmlFor='sample-region'>
              {t('Sample channel region')}
            </FieldLabel>
            <Input
              id='sample-region'
              value={sample.region}
              maxLength={200}
              placeholder='swedencentral'
              onChange={(event) =>
                setSample({ ...sample, region: event.target.value })
              }
            />
          </Field>
        </FieldGroup>
        <div role='status' aria-live='polite' className='flex flex-col gap-3'>
          <p className='text-sm font-medium'>{t('Simulated target groups')}</p>
          <div className='flex flex-wrap gap-2'>
            {result.groups.map((group) => (
              <Badge
                key={group}
                className='max-w-full break-all whitespace-normal'
              >
                {group}
              </Badge>
            ))}
            {!result.groups.length && (
              <p className='text-muted-foreground text-sm'>
                {t('No group matched; no simulated dispatch.')}
              </p>
            )}
          </div>
          <ol className='flex flex-col gap-2'>
            {result.rows.map((row, index) => (
              <li
                key={row.id}
                className='flex flex-col gap-2 rounded-lg border p-3'
              >
                <div className='flex flex-wrap items-start justify-between gap-2'>
                  <span className='min-w-0 text-sm break-words'>
                    {t('Rule {{number}}', { number: index + 1 })} ·{' '}
                    {props.draft.rules[index].name}
                  </span>
                  <Badge
                    variant={row.status === 'matched' ? 'default' : 'outline'}
                  >
                    {statuses[row.status]}
                  </Badge>
                </div>
                {row.conditions.length > 0 && (
                  <p className='text-muted-foreground text-xs'>
                    {row.conditions
                      .map((value, conditionIndex) => {
                        let status = t('Not matched')
                        if (value === null) status = t('Unknown')
                        if (value === true) status = t('Matched')
                        return t('Condition {{number}}: {{status}}', {
                          number: conditionIndex + 1,
                          status,
                        })
                      })
                      .join(' · ')}
                  </p>
                )}
              </li>
            ))}
          </ol>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Regions ignore case. Separate multiple terms with commas; exclusions require all terms to be absent. Unknown values never count as a match.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
