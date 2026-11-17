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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { AzureQuotaBadge } from '../azure-quota-badge'

test('PTUM quota without TPM is identified instead of displaying only a region', async () => {
  render(
    <AzureQuotaBadge
      meta={JSON.stringify([
        { model: 'gpt-6.1-sol', offer: 'ptum', region: 'eastus2' },
      ])}
    />
  )
  expect(screen.getByText('PTU')).toBeVisible()
  expect(screen.queryByText(/Tier [1-6]/)).not.toBeInTheDocument()
  await userEvent.setup().tab()
  expect(
    await screen.findByText(
      'Provisioned throughput (PTU); Global Tier cannot be inferred from this response.'
    )
  ).toBeVisible()
})

test('region-only quota shows why its tier is unknown', async () => {
  render(
    <AzureQuotaBadge
      meta={JSON.stringify([
        { model: 'gpt-6-sol', region: 'eastus2', status: 403, err: 'HTTP 403' },
      ])}
    />
  )
  expect(screen.getByText('Tier unknown')).toBeVisible()
  await userEvent.setup().tab()
  expect(await screen.findByText('HTTP 403')).toBeVisible()
})

test('mixed deployments retain the lowest known tier and identify PTU separately', () => {
  render(
    <AzureQuotaBadge
      meta={JSON.stringify([
        { model: 'a', tier: 6 },
        { model: 'b', tier: 2 },
        { model: 'c', offer: 'ptum' },
      ])}
    />
  )
  expect(screen.getByText('Tier 2')).toBeVisible()
  expect(screen.getByText('PTU')).toBeVisible()
})

test.each([null, '', 'invalid', '[null, false, 123]'])(
  'empty or invalid metadata %s stays pending',
  (meta) => {
    render(<AzureQuotaBadge meta={meta} />)
    expect(screen.getByText('Tier pending')).toBeVisible()
  }
)

test('legacy quota objects still display their tier and region', () => {
  render(
    <AzureQuotaBadge meta={JSON.stringify({ tier: 1, region: 'eastus2' })} />
  )
  expect(screen.getByText('Tier 1')).toBeVisible()
  expect(screen.getByText('eastus2')).toBeVisible()
})
