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
import { expect, test } from 'vitest'

import {
  formatDispatchLabel,
  isRepushable,
  parseDispatchInfo,
} from '../dispatch-info'

test('label shows downstream, group and downstream channel id so same-site channels are distinguishable', () => {
  expect(
    formatDispatchLabel({ ds: 'noahgrape', g: 'azure', rid: 149634, st: 'ok', usd: 0 })
  ).toBe('noahgrape/azure #149634:ok $0')
  expect(
    formatDispatchLabel({ ds: 'noahgrape', g: 'anthropic', st: 'failed' })
  ).toBe('noahgrape/anthropic:failed')
  expect(formatDispatchLabel({ st: 'waiting' })).toBe('waiting')
})

test.each([
  [{ ds: 'noahgrape', g: 'azure', st: 'dead' }, true],
  [{ ds: 'noahgrape', g: 'anthropic', st: 'failed' }, true],
  [{ ds: 'noahgrape', g: 'azure', st: 'ok' }, true],
  [{ ds: 'noahgrape', g: 'azure', st: 'pending' }, false],
  [{ ds: 'noahgrape', g: 'azure', st: 'ok', old: 1 }, true],
  [{ st: 'waiting' }, false],
])('re-push availability for %o is %s', (entry, expected) => {
  expect(isRepushable(entry)).toBe(expected)
})

test('non-array or invalid dispatch_info is not parsed as entries', () => {
  expect(parseDispatchInfo('noahgrape:ok')).toBeNull()
  expect(parseDispatchInfo('{"ds":"x"}')).toBeNull()
  expect(parseDispatchInfo('[{"ds":"x","st":"ok"}]')).toEqual([{ ds: 'x', st: 'ok' }])
})
