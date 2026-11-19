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
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { formatNumber } from '@/lib/format'

import { SettingsSection } from '../components/settings-section'
import {
  getProxyPool,
  proxyPoolSchema,
  saveProxyPool,
  testProxy,
  type ProxyPoolValues,
} from './proxy-pool-api'

const queryKey = ['system-settings', 'dispatch-proxy-pool']

function ProxyTestButton(props: { proxy: string }) {
  const { t } = useTranslation()
  const test = useMutation({ mutationFn: testProxy, gcTime: 0, retry: false })
  const result = test.variables === props.proxy ? test.data : undefined
  return (
    <div className='space-y-1 self-end'>
      <Button
        type='button'
        variant='outline'
        disabled={test.isPending || !props.proxy.trim()}
        onClick={() => test.mutate(props.proxy)}
      >
        {test.isPending ? t('Testing...') : t('Test proxy')}
      </Button>
      <p role='status' className='text-muted-foreground max-w-64 text-xs'>
        {result && (
          <>
            {t(result.message)} · {formatNumber(result.latency_ms)} ms
            {result.status_code > 0 && ` · HTTP ${result.status_code}`}
          </>
        )}
        {test.isError && t('Proxy test request failed')}
      </p>
    </div>
  )
}

export function ProxyPoolSection() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey,
    queryFn: getProxyPool,
    gcTime: 0,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    retry: false,
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Failed to load proxy pool')}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return <ProxyPoolForm key={query.data.revision} pool={query.data} />
}

function ProxyPoolForm(props: { pool: ProxyPoolValues }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)
  const [reloadOpen, setReloadOpen] = useState(false)
  const form = useForm<ProxyPoolValues>({
    resolver: zodResolver(proxyPoolSchema),
    defaultValues: props.pool,
  })
  const rows = useFieldArray({
    control: form.control,
    name: 'proxies',
    keyName: 'formKey',
  })
  const save = useMutation({
    mutationFn: saveProxyPool,
    gcTime: 0,
    onSuccess: (pool) => {
      form.reset(pool)
      queryClient.setQueryData(queryKey, pool)
      toast.success(t('Setting saved'))
    },
  })
  const reload = () => {
    setReloadOpen(false)
    form.reset(props.pool)
    void queryClient.invalidateQueries({ queryKey })
  }

  return (
    <SettingsSection title={t('Channel proxy pool')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Only Noahgrape uses a random enabled proxy from this pool. Other downstreams keep the original channel proxy. Changes apply to new deliveries and repushes; existing downstream channels are unchanged.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'An empty or unavailable pool pauses new Noahgrape deliveries. Retries reuse the selected proxy while it remains enabled. Pool changes take effect within 30 seconds.'
        )}
      </p>
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit((values) => save.mutate(values))}
          className='space-y-4'
        >
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={save.isPending || rows.fields.length >= 500}
              onClick={() =>
                rows.append({ id: '', name: '', proxy: '', enabled: true })
              }
            >
              {t('Add proxy')}
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
          <p className='text-muted-foreground text-sm'>
            {t(
              'Test connects to Google through this proxy from the server. Unsaved proxies can also be tested; testing does not change the enabled state.'
            )}
          </p>
          {rows.fields.length === 0 && (
            <p className='text-muted-foreground py-6 text-sm'>
              {t(
                'No proxies. Add and enable a proxy to resume Noahgrape deliveries.'
              )}
            </p>
          )}
          {rows.fields.map((row, index) => (
            <div
              key={row.formKey}
              className='grid gap-4 rounded-lg border p-4 md:grid-cols-[1fr_2fr_auto_auto_auto]'
            >
              <FormField
                control={form.control}
                name={`proxies.${index}.name`}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Remark')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={save.isPending}
                        maxLength={100}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`proxies.${index}.proxy`}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Proxy URL')}</FormLabel>
                    <FormControl>
                      <PasswordInput
                        {...field}
                        disabled={save.isPending}
                        autoComplete='off'
                        placeholder='socks5://user:password@host:1080'
                        maxLength={2048}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`proxies.${index}.enabled`}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Enabled')}</FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={save.isPending}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <ProxyTestButton proxy={form.watch(`proxies.${index}.proxy`)} />
              <Button
                type='button'
                variant='outline'
                className='self-end'
                disabled={save.isPending}
                onClick={() => setDeleteIndex(index)}
              >
                {t('Delete')}
              </Button>
            </div>
          ))}
        </form>
      </Form>
      <ConfirmDialog
        open={deleteIndex !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteIndex(null)
        }}
        title={t('Delete proxy')}
        desc={t(
          'This removes the proxy from future selections after saving. Existing downstream channels are unchanged.'
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
        desc={t('Discard unsaved changes and reload the proxy pool?')}
        handleConfirm={reload}
      />
    </SettingsSection>
  )
}
