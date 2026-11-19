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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ProxyPoolSection } from './proxy-pool-section'

const pool = {
  revision: 'original',
  proxies: [
    {
      id: 'one',
      name: 'egress',
      proxy: 'socks5://user:secret@host:1080',
      enabled: false,
    },
  ],
}

async function renderPool() {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ProxyPoolSection />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByLabelText('Proxy URL')
}

describe('Proxy pool settings', () => {
  it('tests the unsaved proxy without saving or enabling it', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: pool },
    })
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          reachable: true,
          latency_ms: 123,
          status_code: 204,
          message: 'Proxy connection successful',
        },
      },
    })
    await renderPool()
    const input = screen.getByLabelText('Proxy URL')
    await userEvent.clear(input)
    await userEvent.type(input, 'socks5h://new:password@other:1080')
    await userEvent.click(screen.getByRole('button', { name: 'Test proxy' }))
    await screen.findByText(/Proxy connection successful/)
    expect(post).toHaveBeenCalledWith(
      '/api/proxy-pool/test',
      { proxy: 'socks5h://new:password@other:1080' },
      { timeout: 15000 }
    )
    expect(screen.getByRole('switch')).not.toBeChecked()
    await userEvent.type(input, '0')
    expect(
      screen.queryByText(/Proxy connection successful/)
    ).not.toBeInTheDocument()
  })
  it('masks credentials and saves the enabled state with the original revision', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: pool },
    })
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, data: { ...pool, revision: 'saved' } },
    })
    await renderPool()
    expect(screen.getByLabelText('Proxy URL')).toHaveAttribute(
      'type',
      'password'
    )
    expect(screen.getByRole('switch')).not.toBeChecked()
    await userEvent.click(screen.getByRole('switch'))
    await userEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/proxy-pool/', {
        ...pool,
        proxies: [{ ...pool.proxies[0], enabled: true }],
      })
    )
  })

  it('reload discards edits even when the saved revision has not changed', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: pool },
    })
    await renderPool()
    const name = screen.getByLabelText('Remark')
    await userEvent.clear(name)
    await userEvent.type(name, 'unsaved')
    await userEvent.click(screen.getByRole('button', { name: 'Reload' }))
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() =>
      expect(screen.getByLabelText('Remark')).toHaveValue('egress')
    )
    expect(screen.getByRole('button', { name: 'Save Settings' })).toBeDisabled()
  })
})
