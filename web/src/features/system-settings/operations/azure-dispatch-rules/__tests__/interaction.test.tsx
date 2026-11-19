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
import {
  fireEvent,
  render,
  screen,
  within,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AzureRulesWorkbench } from '../index'
import { conditionDragType } from '../lib'

beforeEach(() => localStorage.clear())
afterEach(() => vi.unstubAllGlobals())

describe('Azure rule builder interactions', () => {
  it('collapses rule editors for compact sorting and keeps their edits when reopened', async () => {
    render(<AzureRulesWorkbench owner={1} />)
    const toggle = screen.getByRole('button', { name: 'Toggle Rule 1 editor' })
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByDisplayValue('High Tier in Europe')
    ).not.toBeInTheDocument()
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByDisplayValue('High Tier in Europe')).toBeVisible()
  })

  it('moves a rule by keyboard and updates the simulated destination', async () => {
    render(<AzureRulesWorkbench owner={1} />)
    const order = screen.getByRole('list', { name: 'Dispatch rule order' })
    expect(
      within(order).getByRole('button', { name: 'Move Rule 1 up' })
    ).toBeDisabled()
    screen.getByRole('button', { name: 'Drag Rule 2 to reorder' }).focus()
    await userEvent.keyboard('{ArrowUp}')
    expect(screen.getAllByLabelText('Rule name')[0]).toHaveValue(
      'General Azure fallback'
    )
    expect(
      screen.getByText('Simulated target groups').parentElement
    ).toHaveTextContent('azure-general')
  })
  it('adds an editable region condition by dropping a palette block', () => {
    render(<AzureRulesWorkbench owner={1} />)
    const target = screen.getByRole('group', { name: 'Conditions for Rule 2' })
    expect(
      fireEvent.dragEnter(target, {
        dataTransfer: { types: [conditionDragType] },
      })
    ).toBe(false)
    fireEvent.drop(target, {
      dataTransfer: {
        getData: (type: string) => (type === conditionDragType ? 'region' : ''),
      },
    })
    expect(within(target).getByLabelText('Condition 2 regions')).toHaveValue('')
    expect(
      within(target).getByLabelText('Condition 2 regions')
    ).toHaveAttribute('aria-invalid', 'true')
  })
  it('offers a click fallback for adding blocks and edits AND to OR', async () => {
    render(<AzureRulesWorkbench owner={1} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'Region condition block' })
    )
    expect(screen.getAllByLabelText('Rule name')).toHaveLength(3)
    const target = screen.getByRole('group', { name: 'Conditions for Rule 3' })
    await userEvent.type(
      within(target).getByLabelText('Condition 1 regions'),
      'sweden'
    )
    await userEvent.selectOptions(
      screen.getAllByLabelText('When these conditions match')[2],
      'any'
    )
    expect(
      screen.getAllByLabelText('When these conditions match')[2]
    ).toHaveValue('any')
  })
  it('duplicates independently and leaves the original unchanged when editing the copy', async () => {
    render(<AzureRulesWorkbench owner={1} />)
    await userEvent.click(
      screen.getAllByRole('button', { name: 'Duplicate rule' })[0]
    )
    const copy = screen.getAllByLabelText('Rule name')[1]
    await userEvent.clear(copy)
    await userEvent.type(copy, 'Different route')
    expect(screen.getAllByLabelText('Rule name')[0]).toHaveValue(
      'High Tier in Europe'
    )
    expect(copy).toHaveValue('Different route')
  })
  it('saves locally, restores on remount and separates different account drafts', async () => {
    const view = render(<AzureRulesWorkbench owner={1} />)
    const name = screen.getAllByLabelText('Rule name')[0]
    await userEvent.clear(name)
    await userEvent.type(name, 'Saved route')
    await userEvent.click(
      screen.getByRole('button', { name: 'Save local draft' })
    )
    await screen.findByText('Local draft saved. Nothing was dispatched.')
    view.unmount()
    const restored = render(<AzureRulesWorkbench owner={1} />)
    expect(screen.getAllByLabelText('Rule name')[0]).toHaveValue('Saved route')
    restored.unmount()
    render(<AzureRulesWorkbench owner={2} />)
    expect(screen.getAllByLabelText('Rule name')[0]).toHaveValue(
      'High Tier in Europe'
    )
  })
  it('retains working edits and reports storage failure instead of claiming a save', async () => {
    render(
      <>
        <Toaster />
        <AzureRulesWorkbench owner={1} />
      </>
    )
    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {
        throw new Error('quota')
      },
    })
    await userEvent.clear(screen.getAllByLabelText('Rule name')[0])
    await userEvent.type(screen.getAllByLabelText('Rule name')[0], 'Keep this')
    await userEvent.click(
      screen.getByRole('button', { name: 'Save local draft' })
    )
    await screen.findByText(
      'Browser storage is unavailable. Keep this page open to retain your draft.'
    )
    expect(screen.getAllByLabelText('Rule name')[0]).toHaveValue('Keep this')
  })
  it('allows cancellation of deletion and renders an empty state after deleting all rules', async () => {
    render(<AzureRulesWorkbench owner={1} />)
    await userEvent.click(screen.getByRole('button', { name: 'Remove Rule 1' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.getAllByLabelText('Rule name')).toHaveLength(2)
    for (let index = 0; index < 2; index += 1) {
      await userEvent.click(
        screen.getByRole('button', { name: 'Remove Rule 1' })
      )
      await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
      await waitFor(() =>
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      )
    }
    expect(screen.getByText('No rules yet')).toBeInTheDocument()
    expect(
      screen.getByText('No group matched; no simulated dispatch.')
    ).toBeInTheDocument()
  })
  it('reports a corrupt saved draft and loads usable examples', () => {
    localStorage.setItem('azure-dispatch-rules-prototype-v1:1', '{invalid')
    render(<AzureRulesWorkbench owner={1} />)
    expect(
      screen.getByText(
        'The local draft could not be read. Examples are shown; save to replace it.'
      )
    ).toBeInTheDocument()
    expect(screen.getAllByLabelText('Rule name')).toHaveLength(2)
  })
})
