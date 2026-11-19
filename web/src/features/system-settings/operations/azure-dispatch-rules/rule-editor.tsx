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
import { ChevronDown, ChevronUp, Copy, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import { MultiSelect } from '@/components/multi-select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

import { ConditionEditor } from './condition-editor'
import {
  conditionDragType,
  newCondition,
  ruleIsComplete,
  sampleGroups,
  type Rule,
  type Condition,
} from './lib'

export function RuleEditor(props: {
  rule: Rule
  index: number
  count: number
  onChange: (rule: Rule) => void
  onMove: (index: number, direction: 'up' | 'down') => void
  onRemove: (id: string) => void
  onDuplicate: () => void
}) {
  const { t } = useTranslation()
  const [over, setOver] = useState(false)
  const [expanded, setExpanded] = useState(true)
  const rule = props.rule
  const label = t('Rule {{number}}', { number: props.index + 1 })
  const addCondition = (field: Condition['field']) => {
    if (rule.conditions.length < 20) {
      props.onChange({
        ...rule,
        conditions: [...rule.conditions, newCondition(field)],
      })
    }
  }
  return (
    <AutoGroupOrderItem
      group={rule.id}
      label={label}
      index={props.index}
      count={props.count}
      onMove={props.onMove}
      onRemove={props.onRemove}
      leading={
        <Switch
          aria-label={t('Enable {{rule}}', { rule: label })}
          checked={rule.enabled}
          onCheckedChange={(enabled) => props.onChange({ ...rule, enabled })}
        />
      }
      body={
        <Collapsible open={expanded}>
          <CollapsibleContent id={`${rule.id}-editor`}>
            <div
              className={cn(
                'mt-3 flex min-w-0 flex-col gap-4 p-2',
                !rule.enabled && 'opacity-60'
              )}
            >
              <FieldGroup className='gap-4'>
                <Field>
                  <FieldLabel htmlFor={`${rule.id}-name`}>
                    {t('Rule name')}
                  </FieldLabel>
                  <Input
                    id={`${rule.id}-name`}
                    value={rule.name}
                    maxLength={80}
                    onChange={(event) =>
                      props.onChange({ ...rule, name: event.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.id}-match`}>
                    {t('When these conditions match')}
                  </FieldLabel>
                  <NativeSelect
                    id={`${rule.id}-match`}
                    value={rule.match}
                    onChange={(event) =>
                      props.onChange({
                        ...rule,
                        match: event.target.value as Rule['match'],
                      })
                    }
                  >
                    <NativeSelectOption value='all'>
                      {t('All conditions (AND)')}
                    </NativeSelectOption>
                    <NativeSelectOption value='any'>
                      {t('Any condition (OR)')}
                    </NativeSelectOption>
                  </NativeSelect>
                </Field>
              </FieldGroup>
              <div
                role='group'
                aria-label={t('Conditions for {{rule}}', { rule: label })}
                className={cn(
                  'flex flex-col gap-2 rounded-xl border-2 border-dashed p-3 transition-colors',
                  over ? 'border-primary bg-primary/5' : 'border-border'
                )}
                onDragEnter={(event) => {
                  if (event.dataTransfer.types.includes(conditionDragType)) {
                    event.preventDefault()
                    setOver(true)
                  }
                }}
                onDragOver={(event) => {
                  if (event.dataTransfer.types.includes(conditionDragType)) {
                    event.preventDefault()
                    event.dataTransfer.dropEffect = 'copy'
                    setOver(true)
                  }
                }}
                onDragLeave={(event) => {
                  if (
                    !(
                      event.relatedTarget instanceof Node &&
                      event.currentTarget.contains(event.relatedTarget)
                    )
                  ) {
                    setOver(false)
                  }
                }}
                onDrop={(event) => {
                  event.preventDefault()
                  setOver(false)
                  const field = event.dataTransfer.getData(conditionDragType)
                  if (field === 'tier' || field === 'region') {
                    addCondition(field)
                  }
                }}
              >
                {rule.conditions.map((condition, index) => (
                  <ConditionEditor
                    key={condition.id}
                    condition={condition}
                    index={index}
                    onChange={(next) =>
                      props.onChange({
                        ...rule,
                        conditions: rule.conditions.map((item) =>
                          item.id === next.id ? next : item
                        ),
                      })
                    }
                    onRemove={() =>
                      props.onChange({
                        ...rule,
                        conditions: rule.conditions.filter(
                          (item) => item.id !== condition.id
                        ),
                      })
                    }
                  />
                ))}
                <p className='text-muted-foreground text-xs'>
                  {t('Drop a condition block here, or add one below.')}
                </p>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={rule.conditions.length >= 20}
                    onClick={() => addCondition('tier')}
                  >
                    <Plus aria-hidden='true' />
                    {t('Add Tier condition')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={rule.conditions.length >= 20}
                    onClick={() => addCondition('region')}
                  >
                    <Plus aria-hidden='true' />
                    {t('Add region condition')}
                  </Button>
                </div>
              </div>
              <Field>
                <FieldLabel htmlFor={`${rule.id}-groups`}>
                  {t('Then dispatch to groups (simulation)')}
                </FieldLabel>
                <MultiSelect
                  id={`${rule.id}-groups`}
                  options={[...new Set([...sampleGroups, ...rule.groups])].map(
                    (group) => ({ value: group, label: group })
                  )}
                  selected={rule.groups}
                  onChange={(groups) =>
                    props.onChange({
                      ...rule,
                      groups: groups
                        .slice(0, 20)
                        .map((group) => group.slice(0, 80)),
                    })
                  }
                  allowCreate
                  placeholder={t('Choose or enter a sample group')}
                />
              </Field>
              {!ruleIsComplete(rule) && (
                <p className='text-destructive text-xs' role='status'>
                  {t(
                    'Complete the name, conditions and target groups to simulate this rule.'
                  )}
                </p>
              )}
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='self-start'
                disabled={props.count >= 50}
                onClick={props.onDuplicate}
              >
                <Copy aria-hidden='true' />
                {t('Duplicate rule')}
              </Button>
            </div>
          </CollapsibleContent>
        </Collapsible>
      }
    >
      <span
        className='text-muted-foreground min-w-0 flex-1 truncate text-xs'
        title={rule.name}
      >
        {rule.name}
      </span>
      <Button
        type='button'
        variant='ghost'
        size='icon-sm'
        aria-expanded={expanded}
        aria-controls={`${rule.id}-editor`}
        aria-label={t('Toggle {{rule}} editor', { rule: label })}
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? (
          <ChevronUp aria-hidden='true' />
        ) : (
          <ChevronDown aria-hidden='true' />
        )}
      </Button>
      {!rule.enabled && <Badge variant='outline'>{t('Disabled')}</Badge>}
    </AutoGroupOrderItem>
  )
}
