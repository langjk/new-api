package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManualChannelTestAppliesDisablePolicy(t *testing.T) {
	initModelListColumnNames(t)
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldCache, oldRedis := common.MemoryCacheEnabled, common.RedisEnabled
	oldDisable, oldEnable := common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled
	oldThreshold := common.ChannelDisableThreshold
	oldCodes, oldKeywords := operation_setting.AutomaticDisableStatusCodeRanges, operation_setting.AutomaticDisableKeywords
	oldRatios, oldGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.MemoryCacheEnabled, common.RedisEnabled = oldCache, oldRedis
		common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = oldDisable, oldEnable
		common.ChannelDisableThreshold = oldThreshold
		operation_setting.AutomaticDisableStatusCodeRanges, operation_setting.AutomaticDisableKeywords = oldCodes, oldKeywords
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
	})
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	common.ChannelDisableThreshold = 0
	common.AutomaticEnableChannelEnabled = true
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	engines := map[string]gorm.Dialector{"sqlite": sqlite.Open(":memory:")}
	if dsn := os.Getenv("TEST_MANUAL_DISABLE_MYSQL_DSN"); dsn != "" {
		engines["mysql"] = mysql.Open(dsn)
	}
	if dsn := os.Getenv("TEST_MANUAL_DISABLE_POSTGRES_DSN"); dsn != "" {
		engines["postgres"] = postgres.Open(dsn)
	}
	for name, dialect := range engines {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			model.DB, model.LOG_DB = db, db
			switch name {
			case "mysql":
				common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
			case "postgres":
				common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
			default:
				common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			}
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Option{}, &model.Log{}))
			user := model.User{Username: "manual-disable-test", Group: "default", Status: common.UserStatusEnabled, Quota: 10000000}
			require.NoError(t, db.Create(&user).Error)
			defer db.Delete(&user)
			for _, tc := range []struct {
				name                                                                     string
				status                                                                   int
				globalOff, channelOff, alreadyDisabled, cancelled, unsupported, match429 bool
				message                                                                  string
				wantDisabled                                                             bool
			}{
				{name: "matching status", status: 401, wantDisabled: true},
				{name: "matching keyword", status: 400, message: "credit exhausted", wantDisabled: true},
				{name: "unmatched rate limit", status: 429},
				{name: "configured rate limit", status: 429, match429: true, wantDisabled: true},
				{name: "temporary timeout", status: 504},
				{name: "manual check independent of global relay switch", status: 401, globalOff: true, wantDisabled: true},
				{name: "channel switch off", status: 401, channelOff: true},
				{name: "manual disable preserved", status: 401, alreadyDisabled: true},
				{name: "success never enables", status: 200, alreadyDisabled: true},
				{name: "success stays enabled", status: 200},
				{name: "cancelled request", status: 401, cancelled: true},
				{name: "unsupported local test", unsupported: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					common.AutomaticDisableChannelEnabled = !tc.globalOff
					operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}
					if tc.match429 {
						operation_setting.AutomaticDisableStatusCodeRanges = append(operation_setting.AutomaticDisableStatusCodeRanges, operation_setting.StatusCodeRange{Start: 429, End: 429})
					}
					operation_setting.AutomaticDisableKeywords = []string{"credit exhausted"}
					requestCtx, cancel := context.WithCancel(context.Background())
					defer cancel()
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if tc.cancelled {
							cancel()
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tc.status)
						if tc.status == 200 {
							fmt.Fprint(w, `{"id":"test","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
						} else {
							fmt.Fprintf(w, `{"error":{"message":"upstream rejection %s","type":"invalid_request_error"}}`, tc.message)
						}
					}))
					defer upstream.Close()
					autoBan := 1
					if tc.channelOff {
						autoBan = 0
					}
					ch := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "manual-policy", BaseURL: &upstream.URL, Key: "test-only", Models: "gpt-4o-mini", Group: "default", Status: common.ChannelStatusEnabled, AutoBan: &autoBan}
					if tc.alreadyDisabled {
						ch.Status = common.ChannelStatusManuallyDisabled
					}
					if tc.unsupported {
						ch.Type = constant.ChannelTypeMidjourney
					}
					require.NoError(t, db.Create(&ch).Error)
					defer db.Delete(&ch)
					ability := model.Ability{ChannelId: ch.Id, Group: "default", Model: "gpt-4o-mini", Enabled: ch.Status == common.ChannelStatusEnabled}
					require.NoError(t, db.Create(&ability).Error)
					defer db.Where("channel_id = ?", ch.Id).Delete(&model.Ability{})
					rec := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(rec)
					ctx.Set("id", user.Id)
					ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(ch.Id)}}
					ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/test/%d?model=gpt-4o-mini&endpoint_type=openai", ch.Id), nil).WithContext(requestCtx)
					TestChannel(ctx)
					wantCalls := int32(1)
					if tc.unsupported {
						wantCalls = 0
					}
					assert.Equal(t, wantCalls, calls.Load(), "no second upstream probe")
					var saved model.Channel
					if tc.status == 200 {
						require.Eventually(t, func() bool { return db.First(&saved, ch.Id).Error == nil && saved.TestTime > 0 }, time.Second, 10*time.Millisecond)
					}
					require.NoError(t, db.First(&saved, ch.Id).Error)
					wantStatus := ch.Status
					if tc.wantDisabled {
						wantStatus = common.ChannelStatusAutoDisabled
					}
					assert.Equal(t, wantStatus, saved.Status, rec.Body.String())
					assert.Equal(t, ch.Key, saved.Key)
					if tc.wantDisabled {
						assert.NotEmpty(t, saved.GetOtherInfo()["status_reason"])
					}
					require.NoError(t, db.Where("channel_id = ?", ch.Id).First(&ability).Error)
					assert.Equal(t, wantStatus == common.ChannelStatusEnabled, ability.Enabled)
				})
			}
		})
	}
}

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
	disable, _ := channelTestDisableDecision(result, 1, 10000, common.AutomaticDisableChannelEnabled)
	assert.False(t, disable, "401 is not hard-coded")
	result.newAPIError.StatusCode = 429
	disable, _ = channelTestDisableDecision(result, 1, 10000, common.AutomaticDisableChannelEnabled)
	assert.True(t, disable)
	operation_setting.AutomaticDisableStatusCodeRanges = nil
	operation_setting.AutomaticDisableKeywords = []string{"test upstream failure"}
	disable, _ = channelTestDisableDecision(result, 1, 10000, common.AutomaticDisableChannelEnabled)
	assert.True(t, disable)
	operation_setting.AutomaticDisableKeywords = nil
	disable, _ = channelTestDisableDecision(result, 1, 10000, common.AutomaticDisableChannelEnabled)
	assert.False(t, disable, "edits take effect without restart")
	disable, _ = channelTestDisableDecision(testResult{}, 10001, 10000, common.AutomaticDisableChannelEnabled)
	assert.True(t, disable, "response time threshold shared")
	common.AutomaticDisableChannelEnabled = false
	disable, _ = channelTestDisableDecision(result, 10001, 10000, common.AutomaticDisableChannelEnabled)
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
		{"image first uses text model", 400, 1, false, false},
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
				channel.Models = "gpt-image-2,gpt-4o-mini"
				endpoint = "/v1/chat/completions"
				body = `{"error":{"type":"invalid_request_error","message":"upstream test rejection"}}`
			}
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "preflight.invalid", r.URL.Host, "request must arrive through the pool proxy")
				assert.Equal(t, endpoint, r.URL.Path)
				if tc.code == 400 {
					var payload map[string]any
					if assert.NoError(t, common.DecodeJson(r.Body, &payload)) {
						assert.Equal(t, "gpt-4o-mini", payload["model"])
						assert.Contains(t, payload, "messages")
						assert.NotContains(t, payload, "prompt")
					}
				}
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
				if tc.code == 400 {
					assert.Equal(t, "gpt-4o-mini", response.Data.Model)
				} else {
					assert.Equal(t, channel.Models, response.Data.Model)
				}
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

func TestPreflightSelectsTextModel(t *testing.T) {
	for _, tc := range []struct{ name, models, test, mapping, want string }{
		{"image first", "gpt-image-2,gpt-6.1-sol", "", "", "gpt-6.1-sol"},
		{"explicit image skipped", "gpt-4o-mini,gpt-image-2", "gpt-image-2", "", "gpt-4o-mini"},
		{"explicit text preferred", "gpt-image-2,gpt-5", "gpt-4o-mini", "", "gpt-4o-mini"},
		{"media skipped", "gpt-audio,gpt-realtime,whisper-1,tts-1,text-embedding-3-small,omni-moderation-latest,sora-2,gpt-3.5-turbo-instruct,gpt-5", "", "", "gpt-5"},
		{"mapped image skipped", "alias,gpt-5", "", `{"alias":"gpt-image-2"}`, "gpt-5"},
		{"mapped text chosen", "image-alias", "", `{"image-alias":"gpt-6.1-sol"}`, "image-alias"},
		{"only images", "gpt-image-2,chatgpt-image-latest,dall-e-3", "", "", ""},
		{"invalid mapping", "gpt-5", "", "invalid", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Models: tc.models, TestModel: &tc.test, ModelMapping: &tc.mapping}
			got, err := preflightTextModel(channel)
			if tc.want == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.models, channel.Models)
		})
	}
}

func TestPreflightPoolFailureNeverDialsOrDisables(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	oldKeywords := operation_setting.AutomaticDisableKeywords
	operation_setting.AutomaticDisableKeywords = []string{"proxy", "connection"}
	t.Cleanup(func() { operation_setting.AutomaticDisableKeywords = oldKeywords })
	for _, scenario := range []string{"empty", "disabled", "invalid proxy", "unreachable proxy", "malformed channel settings", "pool read failure", "only image models"} {
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
			case "only image models":
				channel.Models = "gpt-image-2"
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
