import { expect, test } from 'vitest'

import {
  ANTHROPIC_DEFAULT_MODELS,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
} from '../channel-form'

test.each(['', '  ', 'custom-model'])(
  'Anthropic models %j use defaults only when blank',
  (models) => {
    const form = channelFormSchema.parse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Claude',
      type: 14,
      key: 'test',
      models,
    })
    expect(transformFormDataToCreatePayload(form).channel.models).toBe(
      models.trim() || ANTHROPIC_DEFAULT_MODELS
    )
  }
)
test('other providers still require models', () => {
  expect(
    channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'OpenAI',
      type: 1,
      key: 'test',
      models: '',
    }).success
  ).toBe(false)
})
