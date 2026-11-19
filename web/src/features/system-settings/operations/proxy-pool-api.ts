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
import { z } from 'zod'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export const proxyPoolSchema = z.object({
  revision: z.string(),
  proxies: z
    .array(
      z.object({
        id: z.string(),
        name: z.string().trim().min(1, 'Required').max(100),
        proxy: z
          .string()
          .trim()
          .min(1, 'Required')
          .max(2048)
          .refine((value) => {
            try {
              const url = new URL(value)
              return (
                ['http:', 'https:', 'socks5:', 'socks5h:'].includes(
                  url.protocol
                ) &&
                Boolean(url.hostname) &&
                !url.search &&
                !url.hash &&
                (!url.pathname || url.pathname === '/')
              )
            } catch {
              return false
            }
          }, 'Invalid proxy URL'),
        enabled: z.boolean(),
      })
    )
    .max(500),
})

export type ProxyPoolValues = z.infer<typeof proxyPoolSchema>

export type ProxyTestResult = {
  reachable: boolean
  latency_ms: number
  status_code: number
  message: string
}

export async function testProxy(proxy: string): Promise<ProxyTestResult> {
  const response = await api.post<{
    success: boolean
    message: string
    data: ProxyTestResult
  }>('/api/proxy-pool/test', { proxy }, { timeout: 15000 })
  return requireServerSuccess(response.data).data
}
type ProxyPoolResponse = {
  success: boolean
  message: string
  data: ProxyPoolValues
}

export async function getProxyPool() {
  const response = await api.get<ProxyPoolResponse>('/api/proxy-pool/')
  return requireServerSuccess(response.data).data
}

export async function saveProxyPool(values: ProxyPoolValues) {
  const response = await api.put<ProxyPoolResponse>('/api/proxy-pool/', values)
  return requireServerSuccess(response.data).data
}
