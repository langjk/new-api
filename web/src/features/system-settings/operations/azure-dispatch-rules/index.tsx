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
import { zodResolver } from '@hookform/resolvers/zod'
import { GripVertical, Plus, Save } from 'lucide-react'
import { Reorder } from 'motion/react'
import { nanoid } from 'nanoid'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useAuthStore } from '@/stores/auth-store'
import { createAzureRuleDraftStore } from '@/stores/azure-rule-draft-store'

import { SettingsSection } from '../../components/settings-section'
import {
  conditionDragType,
  draftSchema,
  exampleDraft,
  newRule,
  type Draft,
} from './lib'
import { RulesPreview } from './preview'
import { RuleEditor } from './rule-editor'

export function AzureDispatchRulesSection() {
  const owner = useAuthStore((state) => state.auth.user?.id ?? 'preview')
  return <AzureRulesWorkbench key={owner} owner={owner} />
}

export function AzureRulesWorkbench(props: { owner: number | string }) {
  const { t } = useTranslation()
  const [store] = useState(() => createAzureRuleDraftStore(props.owner))
  const [resetOpen, setResetOpen] = useState(false)
  const [removeId, setRemoveId] = useState<string | null>(null)
  const [announcement, setAnnouncement] = useState('')
  const form = useForm<Draft>({
    resolver: zodResolver(draftSchema),
    defaultValues:
      store.getState().draft ??
      exampleDraft([t('High Tier in Europe'), t('General Azure fallback')]),
  })
  const draft = form.watch()
  const setRules = (rules: Draft['rules']) =>
    form.setValue('rules', rules, { shouldDirty: true })
  const save = form.handleSubmit(
    (values) => {
      try {
        store.getState().save(values)
        form.reset(values)
        setAnnouncement(t('Local draft saved. Nothing was dispatched.'))
        toast.success(t('Local draft saved. Nothing was dispatched.'))
      } catch {
        toast.error(
          t(
            'Browser storage is unavailable. Keep this page open to retain your draft.'
          )
        )
      }
    },
    () => toast.error(t('Draft is invalid or exceeds the prototype limits.'))
  )
  return (
    <SettingsSection title={t('Azure dispatch rules')}>
      <Alert>
        <AlertTitle>{t('Interactive prototype · no live dispatch')}</AlertTitle>
        <AlertDescription>
          {t(
            'Only a draft in this browser. Groups are examples and can be typed freely; no channel, group or dispatch queue is changed.'
          )}
        </AlertDescription>
      </Alert>
      {store.getState().loadFailed && (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'The local draft could not be read. Examples are shown; save to replace it.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <Field className='w-auto'>
          <FieldLabel htmlFor='dispatch-policy'>
            {t('When several rules match')}
          </FieldLabel>
          <NativeSelect
            id='dispatch-policy'
            value={draft.policy}
            onChange={(event) =>
              form.setValue('policy', event.target.value as Draft['policy'], {
                shouldDirty: true,
              })
            }
          >
            <NativeSelectOption value='first'>
              {t('Use the first matching rule')}
            </NativeSelectOption>
            <NativeSelectOption value='all'>
              {t('Merge groups from all matches')}
            </NativeSelectOption>
          </NativeSelect>
        </Field>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-muted-foreground text-xs'>
            {form.formState.isDirty
              ? t('Unsaved local changes')
              : t('Local preview only')}
          </span>
          <Button
            type='button'
            variant='outline'
            onClick={() => setResetOpen(true)}
          >
            {t('Restore examples')}
          </Button>
          <Button type='button' onClick={() => void save()}>
            <Save aria-hidden='true' />
            {t('Save local draft')}
          </Button>
        </div>
      </div>
      <div className='grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_20rem]'>
        <div className='flex min-w-0 flex-col gap-4'>
          <div className='bg-muted/30 flex flex-wrap items-center gap-2 rounded-xl border p-3'>
            <span className='text-sm font-medium'>{t('Condition blocks')}</span>
            {(['tier', 'region'] as const).map((field) => (
              <Button
                key={field}
                type='button'
                variant='outline'
                size='sm'
                draggable
                className='cursor-grab active:cursor-grabbing'
                disabled={draft.rules.length >= 50}
                onDragStart={(event) => {
                  event.dataTransfer.setData(conditionDragType, field)
                  event.dataTransfer.effectAllowed = 'copy'
                }}
                onClick={() => {
                  const rule = newRule(
                    t('Rule {{number}}', { number: draft.rules.length + 1 })
                  )
                  if (field === 'region') {
                    rule.conditions = [
                      {
                        id: nanoid(),
                        field: 'region',
                        operator: 'contains',
                        value: '',
                      },
                    ]
                  }
                  setRules([...draft.rules, rule])
                }}
              >
                <GripVertical aria-hidden='true' />
                {field === 'tier'
                  ? t('Tier condition block')
                  : t('Region condition block')}
              </Button>
            ))}
            <p className='text-muted-foreground w-full text-xs'>
              {t(
                'Drag blocks into a rule. Click a block to create a rule. Drag a rule by its handle to change priority; arrow keys also work.'
              )}
            </p>
          </div>
          <Reorder.Group
            as='ol'
            axis='y'
            values={draft.rules.map((rule) => rule.id)}
            aria-label={t('Dispatch rule order')}
            className='flex flex-col gap-4'
            onReorder={(ids: string[]) => {
              setRules(
                ids.flatMap((id) =>
                  draft.rules.filter((rule) => rule.id === id)
                )
              )
              setAnnouncement(t('Rule order updated.'))
            }}
          >
            {draft.rules.map((rule, index) => (
              <RuleEditor
                key={rule.id}
                rule={rule}
                index={index}
                count={draft.rules.length}
                onChange={(next) =>
                  setRules(
                    draft.rules.map((item) =>
                      item.id === next.id ? next : item
                    )
                  )
                }
                onMove={(from, direction) => {
                  const to = from + (direction === 'up' ? -1 : 1)
                  if (to < 0 || to >= draft.rules.length) return
                  const rules = [...draft.rules]
                  ;[rules[from], rules[to]] = [rules[to], rules[from]]
                  setRules(rules)
                  setAnnouncement(t('Rule order updated.'))
                }}
                onRemove={setRemoveId}
                onDuplicate={() => {
                  const rules = [...draft.rules]
                  rules.splice(index + 1, 0, {
                    ...rule,
                    id: nanoid(),
                    name: t('Copy of {{name}}', { name: rule.name }).slice(
                      0,
                      80
                    ),
                    conditions: rule.conditions.map((condition) => ({
                      ...condition,
                      id: nanoid(),
                    })),
                  })
                  setRules(rules)
                }}
              />
            ))}
          </Reorder.Group>
          {!draft.rules.length && (
            <EmptyState
              className='min-h-40'
              title={t('No rules yet')}
              description={t('Add a rule or click a condition block to begin.')}
            />
          )}
          <Button
            type='button'
            variant='outline'
            className='w-full border-dashed'
            disabled={draft.rules.length >= 50}
            onClick={() =>
              setRules([
                ...draft.rules,
                newRule(
                  t('Rule {{number}}', { number: draft.rules.length + 1 })
                ),
              ])
            }
          >
            <Plus aria-hidden='true' />
            {t('Add rule')}
          </Button>
        </div>
        <RulesPreview draft={draft} />
      </div>
      <p role='status' className='sr-only'>
        {announcement}
      </p>
      <ConfirmDialog
        open={resetOpen}
        onOpenChange={setResetOpen}
        title={t('Restore examples')}
        desc={t(
          'Replace this working draft with example rules? Save afterwards to replace the local draft.'
        )}
        handleConfirm={() => {
          const examples = exampleDraft([
            t('High Tier in Europe'),
            t('General Azure fallback'),
          ])
          setRules(examples.rules)
          form.setValue('policy', examples.policy, { shouldDirty: true })
          setResetOpen(false)
        }}
      />
      <ConfirmDialog
        open={removeId !== null}
        onOpenChange={(open) => {
          if (!open) setRemoveId(null)
        }}
        title={t('Delete rule')}
        desc={t('Remove this rule from the working draft?')}
        destructive
        handleConfirm={() => {
          setRules(draft.rules.filter((rule) => rule.id !== removeId))
          setRemoveId(null)
        }}
      />
    </SettingsSection>
  )
}
