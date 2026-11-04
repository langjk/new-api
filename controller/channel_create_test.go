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
	for _, mode := range []string{"single", "batch", "multi_to_single"} {
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
