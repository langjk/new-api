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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { FieldGroup } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { handleServerError } from '@/lib/handle-server-error'

import { bindDispatchChannel, getDispatchBindings } from '../api'
import { bindingSchema, type BindingValues } from '../lib/channel-form'

export function ChannelDispatchBinding(props: {
  channelId: number
  groups: string[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = ['channel-dispatch-bindings', props.channelId]
  const form = useForm<BindingValues>({
    resolver: zodResolver(bindingSchema),
    defaultValues: { masterGroup: '', remoteId: '' },
  })
  const bindings = useQuery({
    queryKey,
    queryFn: () => getDispatchBindings(props.channelId),
    refetchInterval: (query) =>
      query.state.data?.some((item) => item.status === 'pending')
        ? 2000
        : false,
  })
  const mutation = useMutation({
    mutationFn: (data: BindingValues) =>
      bindDispatchChannel(
        props.channelId,
        data.masterGroup,
        Number(data.remoteId)
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey })
    },
    onError: (error: unknown) => handleServerError(error),
  })
  const submit = form.handleSubmit((data) => mutation.mutate(data))
  const groups = props.groups.filter((group) => group.includes('-'))
  const statusLabels = {
    bound: t('Bound'),
    pending: t('Verifying binding'),
    failed: t('Binding failed'),
  }
  const pending =
    mutation.isPending ||
    bindings.data?.some((item) => item.status === 'pending')
  return (
    <section
      className='border-border/60 rounded-lg border p-4'
      aria-label={t('Bind downstream channel')}
    >
      <h3 className='text-sm font-medium'>{t('Bind downstream channel')}</h3>
      <p className='text-muted-foreground mt-1 mb-4 text-xs'>
        {t(
          'Select the existing downstream channel group and ID. Binding does not create a channel or change this channel group. Root only.'
        )}
      </p>
      <Form {...form}>
        <FieldGroup className='gap-4'>
          <FormField
            control={form.control}
            name='masterGroup'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Downstream group')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={pending || groups.length === 0}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue
                        placeholder={t('Select a registered downstream group')}
                      />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {groups.map((group) => (
                      <SelectItem key={group} value={group}>
                        {group}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='remoteId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Downstream channel ID')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    inputMode='numeric'
                    disabled={pending}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault()
                        event.stopPropagation()
                        if (!pending) void submit()
                      }
                    }}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button
            type='button'
            onClick={() => void submit()}
            disabled={pending || groups.length === 0}
          >
            {pending ? t('Verifying binding') : t('Bind existing channel')}
          </Button>
        </FieldGroup>
      </Form>
      {bindings.isError && (
        <ErrorState
          className='min-h-0'
          onRetry={() => void bindings.refetch()}
        />
      )}
      <div className='mt-3 space-y-2 text-sm' aria-live='polite'>
        {bindings.data?.map((item) => (
          <p key={item.id} className='break-words'>
            {item.master_group} #{item.remote_channel_id}:{' '}
            {statusLabels[item.status]}
            {item.message && <span> — {t(item.message)}</span>}
          </p>
        ))}
      </div>
    </section>
  )
}
