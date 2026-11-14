package controller

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestPreflightDisableRulesIndependentOfGlobalSwitch(t *testing.T) {
	oldEnabled := common.AutomaticDisableChannelEnabled
	oldCodes := operation_setting.AutomaticDisableStatusCodeRanges
	oldKeywords := operation_setting.AutomaticDisableKeywords
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldCodes
		operation_setting.AutomaticDisableKeywords = oldKeywords
	})
	common.AutomaticDisableChannelEnabled = false
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}
	operation_setting.AutomaticDisableKeywords = []string{"your credit balance is too low"}
	for _, tc := range []struct {
		name    string
		status  int
		message string
		want    bool
	}{
		{"invalid key", 401, "invalid x-api-key", true},
		{"no credit", 400, "Your credit balance is too low", true},
		{"temporary timeout", 504, "upstream timeout", false},
		{"wrong protocol", 400, "use responses endpoint", false},
		{"unconfigured error", 400, "You have reached your specified API usage limits", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := types.NewOpenAIError(errors.New(tc.message), types.ErrorCodeBadResponse, tc.status)
			assert.Equal(t, tc.want, service.MatchesChannelDisableRules(err))
			assert.False(t, service.ShouldDisableChannel(err), "relay still honors global switch")
		})
	}
	assert.False(t, service.MatchesChannelDisableRules(nil))
	operation_setting.AutomaticDisableStatusCodeRanges = nil
	err := types.NewOpenAIError(errors.New("invalid x-api-key"), types.ErrorCodeBadResponse, 401)
	assert.False(t, service.MatchesChannelDisableRules(err), "live rule removal takes effect")
}

func TestPreflightPersistsDisableOnlyForMatchingCurrentChannel(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	oldEnabled := common.AutomaticDisableChannelEnabled
	oldCodes := operation_setting.AutomaticDisableStatusCodeRanges
	oldKeywords := operation_setting.AutomaticDisableKeywords
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldCodes
		operation_setting.AutomaticDisableKeywords = oldKeywords
	})
	common.AutomaticDisableChannelEnabled = false
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}
	operation_setting.AutomaticDisableKeywords = nil
	for _, tc := range []struct {
		name            string
		code            int
		autoBan         int
		editDuringProbe bool
		wantDisabled    bool
	}{
		{"invalid key", 401, 1, false, true},
		{"auto ban opt out", 401, 0, false, false},
		{"transient timeout", 504, 1, false, false},
		{"credentials changed during probe", 401, 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.AuditLog{}))
			user := model.User{Id: 987654, Username: "preflight-test", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Setting: `{"accept_unset_model_ratio_model":true}`}
			require.NoError(t, db.Create(&user).Error)
			channel := model.Channel{Type: constant.ChannelTypeAnthropic, Name: "preflight-test", Key: "test-key", Models: "claude-sonnet-4-5", Status: common.ChannelStatusEnabled, AutoBan: &tc.autoBan}
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "/v1/messages", r.URL.Path)
				if tc.editDuringProbe {
					assert.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "replacement-key").Error)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"upstream test rejection"}}`))
			}))
			defer upstream.Close()
			channel.BaseURL = &upstream.URL
			require.NoError(t, db.Create(&channel).Error)
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Set("id", user.Id)
			ctx.Set("role", common.RoleRootUser)
			ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(channel.Id)}}
			ctx.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/channel/%d/preflight", channel.Id), nil)
			PreflightChannel(ctx)
			require.Equal(t, int32(1), requests.Load(), rec.Body.String())
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Outcome string `json:"outcome"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
			require.True(t, response.Success)
			current, err := model.GetChannelById(channel.Id, false)
			require.NoError(t, err)
			if tc.wantDisabled {
				assert.Equal(t, "dead", response.Data.Outcome)
				assert.Equal(t, common.ChannelStatusAutoDisabled, current.Status)
			} else {
				assert.Equal(t, "unknown", response.Data.Outcome)
				assert.Equal(t, common.ChannelStatusEnabled, current.Status)
			}
		})
	}
}
