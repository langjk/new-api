import { expect, test } from 'vitest'

import {
  ANTHROPIC_DEFAULT_MODELS,
  GPT_DEFAULT_MODELS,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

test.each([
  { type: 1, defaults: GPT_DEFAULT_MODELS },
  { type: 14, defaults: ANTHROPIC_DEFAULT_MODELS },
])('channel type $type uses defaults only when blank', ({ type, defaults }) => {
  for (const models of ['', '  ', 'custom-model']) {
    const form = channelFormSchema.parse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Channel',
      type,
      key: 'test',
      models,
    })
    expect(transformFormDataToCreatePayload(form).channel.models).toBe(
      models.trim() || defaults
    )
    expect(transformFormDataToUpdatePayload(form, 123).models).toBe(
      models.trim() || defaults
    )
  }
})
test('other providers still require models', () => {
  expect(
    channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Gemini',
      type: 24,
      key: 'test',
      models: '',
    }).success
  ).toBe(false)
})
