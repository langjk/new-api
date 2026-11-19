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
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import type { Condition } from './lib'

export function ConditionEditor(props: {
  condition: Condition
  index: number
  onChange: (condition: Condition) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const condition = props.condition
  return (
    <div className='bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg border p-2'>
      <Badge variant='secondary'>
        {condition.field === 'tier' ? 'Tier' : t('Region')}
      </Badge>
      {condition.field === 'tier' ? (
        <>
          <NativeSelect
            aria-label={t('Condition {{number}} operator', {
              number: props.index + 1,
            })}
            value={condition.operator}
            onChange={(event) =>
              props.onChange({
                ...condition,
                operator: event.target.value as typeof condition.operator,
              })
            }
          >
            {Object.entries({
              gte: '≥',
              gt: '>',
              eq: '=',
              lt: '<',
              lte: '≤',
            }).map(([value, label]) => (
              <NativeSelectOption key={value} value={value}>
                {label}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <NativeSelect
            aria-label={t('Condition {{number}} tier', {
              number: props.index + 1,
            })}
            value={condition.value}
            onChange={(event) =>
              props.onChange({
                ...condition,
                value: Number(event.target.value),
              })
            }
          >
            {[1, 2, 3, 4, 5, 6].map((tier) => (
              <NativeSelectOption key={tier} value={tier}>
                Tier {tier}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </>
      ) : (
        <>
          <NativeSelect
            aria-label={t('Condition {{number}} operator', {
              number: props.index + 1,
            })}
            value={condition.operator}
            onChange={(event) =>
              props.onChange({
                ...condition,
                operator: event.target.value as typeof condition.operator,
              })
            }
          >
            <NativeSelectOption value='contains'>
              {t('Contains any')}
            </NativeSelectOption>
            <NativeSelectOption value='excludes'>
              {t('Excludes all')}
            </NativeSelectOption>
            <NativeSelectOption value='equals'>
              {t('Equals any')}
            </NativeSelectOption>
          </NativeSelect>
          <Input
            className='min-w-32 flex-1'
            aria-label={t('Condition {{number}} regions', {
              number: props.index + 1,
            })}
            placeholder='europe, sweden'
            value={condition.value}
            maxLength={200}
            aria-invalid={
              !condition.value.split(/[,，]/).some((value) => value.trim())
            }
            onChange={(event) =>
              props.onChange({ ...condition, value: event.target.value })
            }
          />
        </>
      )}
      <Button
        type='button'
        variant='ghost'
        size='icon-sm'
        className='ml-auto'
        aria-label={t('Remove condition {{number}}', {
          number: props.index + 1,
        })}
        onClick={props.onRemove}
      >
        <X aria-hidden='true' />
      </Button>
    </div>
  )
}
