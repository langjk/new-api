package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddChannelForcesAutoBan(t *testing.T) {
	for _, mode := range []string{"single", "batch"} {
		for _, autoBan := range []string{"", `,"auto_ban":null`, `,"auto_ban":0`, `,"auto_ban":1`} {
			t.Run(mode+autoBan, func(t *testing.T) {
				setupTaskPluginBindChannelTest(t)
				key := "test-key"
				expected := 1
				if mode != "single" {
					key = "first-key\nsecond-key"
				}
				if mode == "batch" {
					expected = 2
				}
				body := fmt.Sprintf(`{"mode":%q,"multi_key_mode":"random","channel":{"type":1,"name":"auto-ban-test","key":%q,"models":"gpt","group":"default"%s}}`, mode, key, autoBan)
				response := postAddChannel(t, 1, common.RoleRootUser, body)
				require.Contains(t, response.Body.String(), `"success":true`)
				var channels []model.Channel
				require.NoError(t, model.DB.Find(&channels).Error)
				require.Len(t, channels, expected)
				for _, channel := range channels {
					require.NotNil(t, channel.AutoBan)
					assert.Equal(t, 1, *channel.AutoBan)
				}
			})
		}
	}
}

func TestAnthropicBlankModelsUseDefaults(t *testing.T) {
	channel := &model.Channel{Type: 14, Key: "test-key", Group: "default"}
	require.NoError(t, validateChannel(channel, true))
	assert.Equal(t, "claude-opus-4-8,claude-opus-4-7,claude-opus-4-6,claude-sonnet-5,claude-fable-5,claude-opus-5,claude-fable-5-1,claude-opus-5-5,claude-fable-5-2", channel.Models)
	channel.Models = "custom-model"
	require.NoError(t, validateChannel(channel, true))
	assert.Equal(t, "custom-model", channel.Models)
}

func TestGPTChannelModelDefaults(t *testing.T) {
	for _, models := range []string{"", "  ", "custom-gpt"} {
		channel := &model.Channel{Type: 1, Key: "test-key", Models: models, Group: "default"}
		require.NoError(t, validateChannel(channel, true))
		if models == "custom-gpt" {
			assert.Equal(t, models, channel.Models)
		} else {
			assert.Equal(t, defaultGPTChannelModels, channel.Models)
		}
	}
}

func TestGPTChannelUpdateModelDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields string
		want   string
	}{
		{"omitted", "", "existing-model"},
		{"blank with inferred type", `,"models":" "`, defaultGPTChannelModels},
		{"blank with explicit type", `,"models":"","type":1`, defaultGPTChannelModels},
		{"custom", `,"models":"custom-model"`, "custom-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTaskPluginBindChannelTest(t)
			channel := model.Channel{Type: 1, Key: "test-key", Models: "existing-model", Group: "default", Name: "gpt-default-test", Status: 1}
			require.NoError(t, model.DB.Create(&channel).Error)
			response := putUpdateChannel(t, 1, common.RoleRootUser, fmt.Sprintf(`{"id":%d,"name":"gpt-default-test","group":"default"%s}`, channel.Id, tc.fields))
			require.Contains(t, response.Body.String(), `"success":true`)
			stored, err := model.GetChannelById(channel.Id, false)
			require.NoError(t, err)
			assert.Equal(t, tc.want, stored.Models)
		})
	}
}

func TestRejectUnregisteredChannelGroup(t *testing.T) {
	channel := &model.Channel{Type: 1, Key: "test-key", Models: "gpt", Group: "default,deleted-group"}
	require.Error(t, validateChannel(channel, true))
	channel.Group = "default"
	require.NoError(t, validateChannel(channel, true))
}
