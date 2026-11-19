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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { SettingsSection } from '../components/settings-section'
import {
  azureTierTableSchema,
  getAzureTierTable,
  saveAzureTierTable,
  type AzureTierTable,
} from './azure-tier-api'

const queryKey = ['system-settings', 'azure-tiers']

export function AzureTierSection() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey,
    queryFn: getAzureTierTable,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Failed to load Azure Tier table')}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return <AzureTierForm key={query.data.revision} table={query.data} />
}

function AzureTierForm(props: { table: AzureTierTable }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)
  const [reloadOpen, setReloadOpen] = useState(false)
  const form = useForm<AzureTierTable>({
    resolver: zodResolver(azureTierTableSchema),
    defaultValues: props.table,
  })
  const rows = useFieldArray({ control: form.control, name: 'rows' })
  const values = form.watch('rows')
  const save = useMutation({
    mutationFn: saveAzureTierTable,
    onSuccess: (table) => {
      form.reset(table)
      queryClient.setQueryData(queryKey, table)
      toast.success(t('Setting saved'))
    },
  })
  const reload = () => {
    setReloadOpen(false)
    form.reset(props.table)
    void queryClient.invalidateQueries({ queryKey })
  }
  const data = rows.fields
    .map((row, index) => ({ ...row, index }))
    .filter((row) =>
      (values[row.index]?.model || '')
        .toLowerCase()
        .includes(search.trim().toLowerCase())
    )
  return (
    <SettingsSection title={t('Azure Tier table')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Global Standard TPM thresholds. Enter 0 for an unpublished tier. Equal thresholds use the lowest tier; positive quotas below Tier 1 remain Tier 1. PTU is excluded.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Saving recalculates existing channel tiers from stored TPM within about 60 seconds and affects tier-based dispatch rules. It does not send model requests or enable full scans.'
        )}
      </p>
      <a
        className='text-primary text-sm underline'
        href='https://learn.microsoft.com/en-us/azure/ai-foundry/openai/quotas-limits?view=foundry-classic'
        target='_blank'
        rel='noreferrer'
      >
        {t('Microsoft quota reference (initial table: 2026-11-19)')}
      </a>
      <Form {...form}>
        <form
          className='space-y-4'
          onSubmit={form.handleSubmit((table) => save.mutate(table))}
        >
          <div className='flex flex-wrap gap-2'>
            <Input
              className='w-64'
              aria-label={t('Search models')}
              placeholder={t('Search models')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Button
              type='button'
              variant='outline'
              disabled={save.isPending || rows.fields.length >= 500}
              onClick={() => {
                setSearch('')
                rows.prepend({ model: '', tpm: [0, 0, 0, 0, 0, 0] })
              }}
            >
              {t('Add model')}
            </Button>
            <Button
              type='submit'
              disabled={save.isPending || !form.formState.isDirty}
            >
              {save.isPending ? t('Saving...') : t('Save Settings')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={save.isPending}
              onClick={() =>
                form.formState.isDirty ? setReloadOpen(true) : reload()
              }
            >
              {t('Reload')}
            </Button>
          </div>
          <StaticDataTable
            data={data}
            getRowKey={(row) => row.id}
            emptyContent={t('No models found')}
            columns={[
              {
                id: 'model',
                header: t('Model'),
                className: 'min-w-64',
                cell: (row) => (
                  <FormField
                    control={form.control}
                    name={`rows.${row.index}.model`}
                    render={({ field }) => (
                      <FormItem>
                        <FormControl>
                          <Input
                            {...field}
                            disabled={save.isPending}
                            aria-label={`${t('Model')} ${row.index + 1}`}
                            maxLength={200}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ),
              },
              ...[0, 1, 2, 3, 4, 5].map((tier) => ({
                id: `tier-${tier}`,
                header: `Tier ${tier + 1} TPM`,
                className: 'min-w-36',
                cell: (row: (typeof data)[number]) => (
                  <FormField
                    control={form.control}
                    name={`rows.${row.index}.tpm.${tier}`}
                    render={({ field }) => (
                      <FormItem>
                        <FormControl>
                          <Input
                            {...field}
                            type='number'
                            min={0}
                            step={1}
                            max={1000000000000}
                            disabled={save.isPending}
                            aria-label={`${t('Model')} ${row.index + 1} Tier ${tier + 1} TPM`}
                            onChange={(e) =>
                              field.onChange(
                                e.target.value === ''
                                  ? 0
                                  : Number(e.target.value)
                              )
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ),
              })),
              {
                id: 'actions',
                header: t('Actions'),
                cell: (row) => (
                  <Button
                    type='button'
                    variant='outline'
                    disabled={save.isPending}
                    onClick={() => setDeleteIndex(row.index)}
                  >
                    {t('Delete')}
                  </Button>
                ),
              },
            ]}
          />
        </form>
      </Form>
      <ConfirmDialog
        open={deleteIndex !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteIndex(null)
        }}
        title={t('Delete model')}
        desc={t(
          'After saving, channels for this model will show an unknown Tier. Existing TPM and region data are retained.'
        )}
        destructive
        confirmText={t('Delete')}
        handleConfirm={() => {
          if (deleteIndex !== null) rows.remove(deleteIndex)
          setDeleteIndex(null)
        }}
      />
      <ConfirmDialog
        open={reloadOpen}
        onOpenChange={setReloadOpen}
        title={t('Reload')}
        desc={t('Discard unsaved changes and reload the Tier table?')}
        handleConfirm={reload}
      />
    </SettingsSection>
  )
}
