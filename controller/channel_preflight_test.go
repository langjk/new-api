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
	"strings"
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
		{"non-text model rejection", 400, 1, false, false},
		{"rate limit allowed", 429, 1, false, false},
		{"rate limit matched by live rule", 429, 1, false, true},
		{"proxy authentication failure", 407, 1, false, false},
		{"credentials changed during probe", 401, 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}
			if tc.code == 429 && tc.wantDisabled {
				operation_setting.AutomaticDisableStatusCodeRanges = append(operation_setting.AutomaticDisableStatusCodeRanges, operation_setting.StatusCodeRange{Start: 429, End: 429})
			}
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.AuditLog{}, &model.Option{}))
			user := model.User{Id: 987654, Username: "preflight-test", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Setting: `{"accept_unset_model_ratio_model":true}`}
			require.NoError(t, db.Create(&user).Error)
			channel := model.Channel{Type: constant.ChannelTypeAnthropic, Name: "preflight-test", Key: "test-key", Models: "claude-sonnet-4-5", Status: common.ChannelStatusEnabled, AutoBan: &tc.autoBan}
			endpoint := "/v1/messages"
			body := `{"type":"error","error":{"type":"authentication_error","message":"upstream test rejection"}}`
			if tc.code == 400 {
				channel.Type = constant.ChannelTypeOpenAI
				channel.Models = "gpt-image-1"
				endpoint = "/v1/chat/completions"
				body = `{"error":{"type":"invalid_request_error","code":"unsupported_model","message":"This model is only supported for image generation, not /v1/chat/completions."}}`
			}
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "preflight.invalid", r.URL.Host, "request must arrive through the pool proxy")
				assert.Equal(t, endpoint, r.URL.Path)
				if tc.editDuringProbe {
					assert.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "replacement-key").Error)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			_, err := model.SaveDispatchProxyPool(model.DispatchProxyPool{Proxies: []model.DispatchProxy{
				{Name: "disabled", Proxy: "http://disabled.invalid:8080", Enabled: false},
				{Name: "test proxy", Proxy: upstream.URL, Enabled: true},
			}})
			require.NoError(t, err)
			channel.BaseURL = common.GetPointer("http://preflight.invalid")
			originalSetting := `{"proxy":"http://original.invalid:8080","force_format":true,"future_setting":"preserved"}`
			channel.Setting = &originalSetting
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
					preflightDiagnostic
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
			require.True(t, response.Success)
			if tc.editDuringProbe {
				assert.Empty(t, response.Data.preflightDiagnostic, "old credentials must not leak stale diagnostics")
			} else {
				assert.Contains(t, response.Data.Message, body, "preserve the provider's raw error body")
				assert.Equal(t, "upstream", response.Data.ErrorSource)
				assert.Equal(t, tc.code, response.Data.UpstreamStatus)
				assert.Equal(t, channel.Models, response.Data.Model)
				assert.Equal(t, endpoint, response.Data.Endpoint)
			}
			current, err := model.GetChannelById(channel.Id, false)
			require.NoError(t, err)
			require.NotNil(t, current.Setting)
			assert.JSONEq(t, originalSetting, *current.Setting, "pool proxy must not overwrite stored settings")
			if tc.wantDisabled {
				assert.Equal(t, "dead", response.Data.Outcome)
				assert.Equal(t, common.ChannelStatusAutoDisabled, current.Status)
			} else {
				want := "allowed"
				if tc.editDuringProbe || tc.code == 407 {
					want = "unknown"
				}
				assert.Equal(t, want, response.Data.Outcome)
				assert.Equal(t, common.ChannelStatusEnabled, current.Status)
			}
		})
	}
}

func TestPreflightPoolFailureNeverDialsOrDisables(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	oldKeywords := operation_setting.AutomaticDisableKeywords
	operation_setting.AutomaticDisableKeywords = []string{"proxy", "connection"}
	t.Cleanup(func() { operation_setting.AutomaticDisableKeywords = oldKeywords })
	for _, scenario := range []string{"empty", "disabled", "invalid proxy", "unreachable proxy", "malformed channel settings", "pool read failure"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.AuditLog{}, &model.Option{}))
			user := model.User{Id: 987654, Username: "proxy-failure-test", Status: common.UserStatusEnabled, Group: "default", Setting: `{"accept_unset_model_ratio_model":true}`}
			require.NoError(t, db.Create(&user).Error)
			var directRequests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				directRequests.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer upstream.Close()
			setting := `{"proxy":"` + upstream.URL + `"}`
			channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "fake-preflight-key", Models: "gpt-4o-mini", BaseURL: &upstream.URL, Setting: &setting, Status: common.ChannelStatusEnabled, AutoBan: common.GetPointer(1)}
			pool := model.DispatchProxyPool{Proxies: []model.DispatchProxy{}}
			proxy := model.DispatchProxy{ID: "test-proxy", Name: "test", Proxy: upstream.URL, Enabled: true}
			switch scenario {
			case "disabled":
				proxy.Enabled = false
			case "invalid proxy":
				proxy.Proxy = "invalid"
			case "unreachable proxy":
				closed := httptest.NewServer(http.NotFoundHandler())
				proxy.Proxy = closed.URL
				closed.Close()
			case "malformed channel settings":
				setting = `{"http_protocol":123}`
			}
			if scenario != "empty" {
				pool.Proxies = append(pool.Proxies, proxy)
			}
			encoded, err := common.Marshal(pool)
			require.NoError(t, err)
			require.NoError(t, db.Create(&model.Option{Key: model.DispatchProxyPoolOption, Value: string(encoded)}).Error)
			if scenario == "pool read failure" {
				require.NoError(t, db.Migrator().DropTable(&model.Option{}))
			}
			require.NoError(t, db.Create(&channel).Error)
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Set("id", user.Id)
			ctx.Set("role", common.RoleRootUser)
			ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(channel.Id)}}
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/preflight", nil)
			PreflightChannel(ctx)
			assert.Contains(t, rec.Body.String(), `"outcome":"unknown"`)
			assert.Contains(t, rec.Body.String(), `"error_source":"local"`)
			assert.NotContains(t, rec.Body.String(), "fake-preflight-key")
			assert.NotContains(t, rec.Body.String(), proxy.Proxy)
			assert.Zero(t, directRequests.Load(), "never fall back to direct or the saved channel proxy")
			current, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, common.ChannelStatusEnabled, current.Status)
			assert.Equal(t, setting, *current.Setting)
		})
	}
}

func TestPreflightDiagnosticRedactsCredentialsAndDistinguishesLocalErrors(t *testing.T) {
	key := `secret-key-with-"quote`
	setting := `{"proxy":"socks5://proxy-user:proxy-password@proxy.example:1080"}`
	headers := `{"Authorization":"Bearer override-secret"}`
	channel := &model.Channel{Key: key, Setting: &setting, HeaderOverride: &headers}
	encoded, err := common.Marshal(key)
	require.NoError(t, err)
	message := "unsupported model; " + key + " " + string(encoded) + " proxy-user proxy-password override-secret active-key"
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "active-key")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	for _, status := range []int{0, 400} {
		diagnostic := channelPreflightDiagnostic(channel, testResult{context: ctx, localErr: errors.New(message), upstreamStatus: status})
		assert.Contains(t, diagnostic.Message, "unsupported model")
		assert.Contains(t, diagnostic.Message, "[redacted]")
		for _, secret := range []string{key, "proxy-user", "proxy-password", "override-secret", "active-key", "secret-key-with"} {
			assert.NotContains(t, diagnostic.Message, secret)
		}
		if status == 0 {
			assert.Equal(t, "local", diagnostic.ErrorSource)
			assert.Zero(t, diagnostic.UpstreamStatus, "local errors must not pretend to be upstream HTTP failures")
		} else {
			assert.Equal(t, "upstream", diagnostic.ErrorSource)
		}
	}
	longMessage := strings.Repeat("detail ", 100) + "original error tail"
	diagnostic := channelPreflightDiagnostic(channel, testResult{localErr: errors.New(longMessage)})
	assert.Equal(t, longMessage, diagnostic.Message, "do not discard provider details after 200 characters")
	assert.Empty(t, channelPreflightDiagnostic(channel, testResult{}))
}
