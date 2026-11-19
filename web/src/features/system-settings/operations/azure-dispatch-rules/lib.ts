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
import { nanoid } from 'nanoid'
import { z } from 'zod'

const conditionSchema = z.discriminatedUnion('field', [
  z.object({
    id: z.string(),
    field: z.literal('tier'),
    operator: z.enum(['gte', 'gt', 'eq', 'lt', 'lte']),
    value: z.number().int().min(1).max(6),
  }),
  z.object({
    id: z.string(),
    field: z.literal('region'),
    operator: z.enum(['contains', 'excludes', 'equals']),
    value: z.string().max(200),
  }),
])
export const draftSchema = z
  .object({
    version: z.literal(1),
    policy: z.enum(['first', 'all']),
    rules: z
      .array(
        z.object({
          id: z.string(),
          name: z.string().max(80),
          enabled: z.boolean(),
          match: z.enum(['all', 'any']),
          conditions: z.array(conditionSchema).max(20),
          groups: z.array(z.string().min(1).max(80)).max(20),
        })
      )
      .max(50),
  })
  .refine(
    (draft) =>
      new Set(draft.rules.map((rule) => rule.id)).size === draft.rules.length
  )

export type Draft = z.infer<typeof draftSchema>
export type Rule = Draft['rules'][number]
export type Condition = Rule['conditions'][number]
export type SampleChannel = { tier: number | null; region: string }
export const conditionDragType = 'application/x-azure-rule-condition'
export const sampleGroups = ['azure-high', 'azure-europe', 'azure-general']

export function newCondition(field: Condition['field']): Condition {
  if (field === 'tier') {
    return { id: nanoid(), field, operator: 'gte', value: 3 }
  }
  return { id: nanoid(), field, operator: 'contains', value: '' }
}

export function newRule(name: string): Rule {
  return {
    id: nanoid(),
    name,
    enabled: true,
    match: 'all',
    conditions: [newCondition('tier')],
    groups: [],
  }
}

export function exampleDraft(names: [string, string]): Draft {
  return {
    version: 1,
    policy: 'first',
    rules: [
      {
        ...newRule(names[0]),
        conditions: [
          newCondition('tier'),
          {
            ...newCondition('region'),
            field: 'region',
            operator: 'contains',
            value: 'europe, sweden',
          },
        ],
        groups: ['azure-high'],
      },
      {
        ...newRule(names[1]),
        conditions: [
          { id: nanoid(), field: 'tier', operator: 'gte', value: 1 },
        ],
        groups: ['azure-general'],
      },
    ],
  }
}

export function ruleIsComplete(rule: Rule): boolean {
  return (
    !!rule.name.trim() &&
    rule.conditions.length > 0 &&
    rule.groups.length > 0 &&
    rule.conditions.every(
      (condition) =>
        condition.field !== 'region' ||
        condition.value.split(/[,，]/).some((value) => value.trim())
    )
  )
}

// Preview only: evaluates the entered sample, never reads or changes real channels.
export function evaluateDraft(draft: Draft, sample: SampleChannel) {
  let stopped = false
  const groups = new Set<string>()
  const rows = draft.rules.map((rule) => {
    if (!rule.enabled) {
      return { id: rule.id, status: 'disabled' as const, conditions: [] }
    }
    if (stopped) {
      return { id: rule.id, status: 'skipped' as const, conditions: [] }
    }
    if (!ruleIsComplete(rule)) {
      return { id: rule.id, status: 'incomplete' as const, conditions: [] }
    }
    const conditions = rule.conditions.map((condition): boolean | null => {
      if (condition.field === 'tier') {
        if (sample.tier === null) return null
        switch (condition.operator) {
          case 'gte':
            return sample.tier >= condition.value
          case 'gt':
            return sample.tier > condition.value
          case 'eq':
            return sample.tier === condition.value
          case 'lt':
            return sample.tier < condition.value
          case 'lte':
            return sample.tier <= condition.value
        }
      }
      const region = sample.region.trim().toLowerCase()
      if (!region) return null
      const terms = condition.value
        .toLowerCase()
        .split(/[,，]/)
        .map((value) => value.trim())
        .filter(Boolean)
      if (!terms.length) return null
      if (condition.operator === 'equals') return terms.includes(region)
      const found = terms.some((value) => region.includes(value))
      return condition.operator === 'excludes' ? !found : found
    })
    const matches =
      rule.match === 'all'
        ? conditions.every((value) => value === true)
        : conditions.some((value) => value === true)
    if (matches) {
      rule.groups.forEach((group) => groups.add(group))
      stopped = draft.policy === 'first'
      return { id: rule.id, status: 'matched' as const, conditions }
    }
    const unknown =
      conditions.includes(null) &&
      (rule.match === 'any' || !conditions.includes(false))
    return {
      id: rule.id,
      status: unknown ? ('unknown' as const) : ('unmatched' as const),
      conditions,
    }
  })
  return { rows, groups: [...groups] }
}
