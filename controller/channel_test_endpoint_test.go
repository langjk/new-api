package controller

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAutomaticChannelTestProtocol(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider int
		model    string
		endpoint string
	}{
		{"azure sol61", 3, "gpt-6.1-sol", string(constant.EndpointTypeOpenAIResponse)},
		{"azure sol6", 3, "gpt-6-sol", string(constant.EndpointTypeOpenAIResponse)},
		{"openai sol61", 1, "gpt-6.1-sol", string(constant.EndpointTypeOpenAIResponse)},
		{"aws claude", 33, "us.anthropic.claude-sonnet-4-6", string(constant.EndpointTypeAnthropic)},
		{"aws nova", 33, "amazon.nova-pro-v1:0", ""},
		{"anthropic", 14, "claude-opus-4-8", string(constant.EndpointTypeAnthropic)},
		{"azure existing", 3, "gpt-4o", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.provider}
			endpoint := normalizeChannelTestEndpoint(channel, "", tc.model)
			assert.Equal(t, tc.endpoint, endpoint)
			if endpoint == string(constant.EndpointTypeOpenAIResponse) {
				assert.IsType(t, &dto.OpenAIResponsesRequest{}, buildTestRequest(tc.model, endpoint, channel, false))
			}
			if endpoint == string(constant.EndpointTypeAnthropic) {
				assert.IsType(t, &dto.ClaudeRequest{}, buildTestRequest(tc.model, endpoint, channel, false))
			}
		})
	}
	mapping := `{"alias":"gpt-6.1-sol"}`
	ch := &model.Channel{Type: 3, ModelMapping: &mapping}
	assert.Equal(t, string(constant.EndpointTypeOpenAIResponse), normalizeChannelTestEndpoint(ch, "", "alias"))
	assert.Equal(t, string(constant.EndpointTypeOpenAI), normalizeChannelTestEndpoint(ch, string(constant.EndpointTypeOpenAI), "alias"))
}
