package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChannelTestDisableDecisionReadsLiveRules(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldEnabled := common.AutomaticDisableChannelEnabled
	oldCodes := operation_setting.AutomaticDisableStatusCodeRanges
	oldKeywords := operation_setting.AutomaticDisableKeywords
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldCodes
		operation_setting.AutomaticDisableKeywords = oldKeywords
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableKeywords = nil
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
	result := testResult{newAPIError: types.NewOpenAIError(errors.New("test upstream failure"), types.ErrorCodeBadResponse, 401)}
	disable, _ := channelTestDisableDecision(result, 1, 10000)
	assert.False(t, disable, "401 is not hard-coded")
	result.newAPIError.StatusCode = 429
	disable, _ = channelTestDisableDecision(result, 1, 10000)
	assert.True(t, disable)
	operation_setting.AutomaticDisableStatusCodeRanges = nil
	operation_setting.AutomaticDisableKeywords = []string{"test upstream failure"}
	disable, _ = channelTestDisableDecision(result, 1, 10000)
	assert.True(t, disable)
	operation_setting.AutomaticDisableKeywords = nil
	disable, _ = channelTestDisableDecision(result, 1, 10000)
	assert.False(t, disable, "edits take effect without restart")
	disable, _ = channelTestDisableDecision(testResult{}, 10001, 10000)
	assert.True(t, disable, "response time threshold shared")
	common.AutomaticDisableChannelEnabled = false
	disable, _ = channelTestDisableDecision(result, 10001, 10000)
	assert.False(t, disable)
}
func TestPreflightRequiresRootBeforeTestingOrDisabling(t *testing.T) {
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Set("role", role)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/preflight", nil)
		PreflightChannel(ctx)
		assert.Contains(t, rec.Body.String(), `"success":false`)
	}
}
