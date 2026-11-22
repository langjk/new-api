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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { ChannelDispatchBinding } from '../channel-dispatch-binding'

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { id: 1, status: 'pending' } },
  })
})
afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})
function show(groups = ['default', 'noahgrape-azure']) {
  const outerSubmit = vi.fn((event) => event.preventDefault())
  render(
    <QueryClientProvider client={client}>
      <form onSubmit={outerSubmit}>
        <ChannelDispatchBinding channelId={42} groups={groups} />
      </form>
    </QueryClientProvider>
  )
  return outerSubmit
}
async function selectGroup() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Downstream group' }))
  await user.click(screen.getByRole('option', { name: 'noahgrape-azure' }))
  return user
}
test.each(['button', 'enter'])(
  'binds using %s without submitting channel edits',
  async (mode) => {
    const outer = show()
    const user = await selectGroup()
    await user.type(
      screen.getByRole('textbox', { name: 'Downstream channel ID' }),
      '1234'
    )
    if (mode === 'enter') await user.keyboard('{Enter}')
    else {
      await user.click(
        screen.getByRole('button', { name: 'Bind existing channel' })
      )
    }
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/channel/42/dispatch-bindings',
        { master_group: 'noahgrape-azure', remote_channel_id: 1234 }
      )
    )
    expect(outer).not.toHaveBeenCalled()
  }
)
test.each(['0', '-1', '2.5', '2147483648'])(
  'rejects invalid ID %s',
  async (id) => {
    show()
    const user = await selectGroup()
    await user.type(
      screen.getByRole('textbox', { name: 'Downstream channel ID' }),
      id
    )
    await user.click(
      screen.getByRole('button', { name: 'Bind existing channel' })
    )
    expect(await screen.findByText('Enter a positive channel ID')).toBeVisible()
    expect(api.post).not.toHaveBeenCalled()
  }
)
test.each([
  ['pending', 'Verifying binding'],
  ['bound', 'Bound'],
  ['failed', 'Binding failed'],
])('shows %s request', async (status, label) => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          id: 1,
          master_group: 'noahgrape-azure',
          remote_channel_id: 99,
          status,
          message: '',
        },
      ],
    },
  })
  show()
  expect(await screen.findByText(`noahgrape-azure #99: ${label}`)).toBeVisible()
  if (status === 'pending') {
    expect(
      screen.getByRole('textbox', { name: 'Downstream channel ID' })
    ).toBeDisabled()
  }
})
test('disables binding without a downstream group and offers query retry', async () => {
  vi.mocked(api.get).mockRejectedValueOnce(new Error('offline'))
  show(['default'])
  expect(
    screen.getByRole('button', { name: 'Bind existing channel' })
  ).toBeDisabled()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  )
})
