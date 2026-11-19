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
import { describe, expect, it } from 'vitest'

import { evaluateDraft, exampleDraft, type Draft } from '../lib'

describe('Azure rules preview evaluation', () => {
  it('stops at the first match and respects reordered priority', () => {
    const draft = exampleDraft(['Europe', 'Fallback'])
    expect(
      evaluateDraft(draft, { tier: 4, region: 'SwedenCentral' }).groups
    ).toEqual(['azure-high'])
    draft.rules.reverse()
    expect(
      evaluateDraft(draft, { tier: 4, region: 'SwedenCentral' }).groups
    ).toEqual(['azure-general'])
  })
  it('merges and deduplicates groups when all matches are selected', () => {
    const draft = exampleDraft(['Europe', 'Fallback'])
    draft.policy = 'all'
    draft.rules[1].groups.push('azure-high')
    expect(
      evaluateDraft(draft, { tier: 4, region: 'swedencentral' }).groups
    ).toEqual(['azure-high', 'azure-general'])
  })
  it('requires all AND conditions but accepts any true OR condition', () => {
    const draft = exampleDraft(['Europe', 'Fallback'])
    draft.rules.pop()
    expect(
      evaluateDraft(draft, { tier: 2, region: 'swedencentral' }).groups
    ).toEqual([])
    draft.rules[0].match = 'any'
    expect(
      evaluateDraft(draft, { tier: 2, region: 'swedencentral' }).groups
    ).toEqual(['azure-high'])
  })
  it('does not match unknown tier or unknown region even with an exclusion', () => {
    const draft = exampleDraft(['Europe', 'Fallback'])
    draft.rules.pop()
    draft.rules[0].conditions = [
      {
        id: 'region',
        field: 'region',
        operator: 'excludes',
        value: 'us, china',
      },
    ]
    expect(evaluateDraft(draft, { tier: 4, region: '' }).rows[0].status).toBe(
      'unknown'
    )
    draft.rules[0].conditions = [
      { id: 'tier', field: 'tier', operator: 'lt', value: 3 },
    ]
    expect(
      evaluateDraft(draft, { tier: null, region: 'europe' }).groups
    ).toEqual([])
  })
  it('requires all excluded terms to be absent and exact equals to match a complete region', () => {
    const draft = exampleDraft(['Region', 'Fallback'])
    draft.rules.pop()
    draft.rules[0].conditions = [
      {
        id: 'region',
        field: 'region',
        operator: 'excludes',
        value: 'china, US',
      },
    ]
    expect(evaluateDraft(draft, { tier: 4, region: 'eastus' }).groups).toEqual(
      []
    )
    expect(
      evaluateDraft(draft, { tier: 4, region: 'swedencentral' }).groups
    ).toEqual(['azure-high'])
    draft.rules[0].conditions[0] = {
      id: 'region',
      field: 'region',
      operator: 'equals',
      value: 'europe, swedencentral',
    }
    expect(
      evaluateDraft(draft, { tier: 4, region: 'northeurope' }).groups
    ).toEqual([])
  })
  it.each(['name', 'conditions', 'groups', 'blank-region'] as const)(
    'does not match incomplete %s rules',
    (field) => {
      const draft = exampleDraft(['Europe', 'Fallback'])
      draft.rules.pop()
      if (field === 'name') draft.rules[0].name = ''
      if (field === 'conditions') draft.rules[0].conditions = []
      if (field === 'groups') draft.rules[0].groups = []
      if (field === 'blank-region') {
        draft.rules[0].conditions = [
          {
            id: 'region',
            field: 'region',
            operator: 'excludes',
            value: ', ， ',
          },
        ]
      }
      expect(
        evaluateDraft(draft, { tier: 4, region: 'swedencentral' }).rows[0]
          .status
      ).toBe('incomplete')
    }
  )
  it('ignores disabled rules and safely evaluates an empty draft', () => {
    const draft = exampleDraft(['Europe', 'Fallback'])
    draft.rules[0].enabled = false
    expect(
      evaluateDraft(draft, { tier: 4, region: 'swedencentral' }).groups
    ).toEqual(['azure-general'])
    const empty: Draft = { version: 1, policy: 'all', rules: [] }
    expect(
      evaluateDraft(empty, { tier: 4, region: 'swedencentral' }).groups
    ).toEqual([])
  })
})
