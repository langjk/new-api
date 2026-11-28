package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAzureTierStorage(t *testing.T) {
	engines := map[string]gorm.Dialector{"sqlite": sqlite.Open(":memory:")}
	if dsn := os.Getenv("TEST_AZURE_TIERS_MYSQL_DSN"); dsn != "" {
		engines["mysql"] = mysql.Open(dsn)
	}
	if dsn := os.Getenv("TEST_AZURE_TIERS_POSTGRES_DSN"); dsn != "" {
		engines["postgres"] = postgres.Open(dsn)
	}
	for name, dialect := range engines {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			oldDB := model.DB
			model.DB = db
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { model.DB = oldDB; _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&model.Option{}))
			require.NoError(t, db.Where(&model.Option{Key: model.DispatchAzureTiersOption}).Delete(&model.Option{}).Error)
			initial, err := model.GetAzureTierTable()
			require.NoError(t, err)
			require.Empty(t, initial.Revision)
			found := false
			for _, row := range initial.Rows {
				if row.Model == "gpt-6.2-astra" {
					found = true
					assert.Equal(t, []int64{250000, 1000000, 2500000, 7000000, 10000000, 15000000}, row.TPM)
				}
			}
			require.True(t, found)
			edited := model.AzureTierTable{Rows: []model.AzureTierRow{{Model: " New-Model ", TPM: []int64{100, 200, 0, 400, 400, 600}}}}
			saved, err := model.SaveAzureTierTable(edited)
			require.NoError(t, err)
			require.NotEmpty(t, saved.Revision)
			assert.Equal(t, "new-model", saved.Rows[0].Model)
			// Repeated schema startup on an existing options table preserves saved values.
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.Option{}))
				got, err := model.GetAzureTierTable()
				require.NoError(t, err)
				assert.Equal(t, saved, got)
			}
			_, err = model.SaveAzureTierTable(initial)
			require.ErrorIs(t, err, model.ErrAzureTiersConflict)
			for _, rows := range [][]model.AzureTierRow{
				{{Model: "bad", TPM: []int64{200, 100, 0, 0, 0, 0}}},
				{{Model: "bad", TPM: []int64{0, 0, 0, 0, 0, 0}}},
				{{Model: "bad", TPM: []int64{1, 2}}},
				{{Model: "bad", TPM: []int64{-1, 2, 3, 4, 5, 6}}},
				{{Model: "bad", TPM: []int64{1, 2, 3, 4, 5, 1000000000001}}},
				{{Model: "same", TPM: []int64{1, 2, 3, 4, 5, 6}}, {Model: "SAME", TPM: []int64{1, 2, 3, 4, 5, 6}}},
			} {
				_, err := model.SaveAzureTierTable(model.AzureTierTable{Revision: saved.Revision, Rows: rows})
				require.ErrorIs(t, err, model.ErrAzureTiersInvalid)
			}
			deleted, err := model.SaveAzureTierTable(model.AzureTierTable{Revision: saved.Revision, Rows: []model.AzureTierRow{}})
			require.NoError(t, err)
			require.Empty(t, deleted.Rows)
			got, err := model.GetAzureTierTable()
			require.NoError(t, err)
			require.Empty(t, got.Rows)
			_, err = model.SaveAzureTierTable(saved)
			require.ErrorIs(t, err, model.ErrAzureTiersConflict)
		})
	}
}

func TestAzureTierRootAccessAndValidation(t *testing.T) {
	user, token := setupAccessTokenAudit(t)
	router := gin.New()
	router.GET("/api/azure-tiers/", middleware.RootAuth(), middleware.DisableCache(), GetAzureTierTable)
	router.PUT("/api/azure-tiers/", middleware.RootAuth(), middleware.DisableCache(), UpdateAzureTierTable)
	router.PUT("/api/option/", middleware.RootAuth(), UpdateOption)
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		require.NoError(t, model.DB.Model(user).Update("role", role).Error)
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			r := accessTokenRequest(router, method, "/api/azure-tiers/", token, "", `{"revision":"","rows":[]}`)
			if role == common.RoleRootUser {
				assert.Equal(t, http.StatusOK, r.Code)
				assert.Contains(t, r.Header().Get("Cache-Control"), "no-store")
			} else {
				assert.Equal(t, http.StatusForbidden, r.Code)
			}
		}
	}
	readOnly, _ := createScopedAccessToken(t, user.Id, -1, "option:read")
	assert.Equal(t, http.StatusOK, accessTokenRequest(router, http.MethodGet, "/api/azure-tiers/", readOnly, "", "").Code)
	assert.Equal(t, http.StatusForbidden, accessTokenRequest(router, http.MethodPut, "/api/azure-tiers/", readOnly, "", `{}`).Code)
	assert.Equal(t, http.StatusUnauthorized, accessTokenRequest(router, http.MethodGet, "/api/azure-tiers/", "", "", "").Code)
	assert.Equal(t, http.StatusBadRequest, accessTokenRequest(router, http.MethodPut, "/api/option/", token, "", `{"key":"DispatchAzureTierTable","value":"{}"}`).Code)
	assert.Equal(t, http.StatusBadRequest, accessTokenRequest(router, http.MethodPut, "/api/azure-tiers/", token, "", `{"rows":[{"model":"x","tpm":[0,0,0,0,0,0]}]}`).Code)
	assert.Equal(t, http.StatusConflict, accessTokenRequest(router, http.MethodPut, "/api/azure-tiers/", token, "", `{"revision":"stale","rows":[]}`).Code)
}

func TestAzureManualMetadataDatabaseMatrix(t *testing.T) {
	engines := map[string]gorm.Dialector{"sqlite": sqlite.Open(":memory:")}
	if dsn := os.Getenv("TEST_AZURE_TIERS_MYSQL_DSN"); dsn != "" {
		engines["mysql"] = mysql.Open(dsn)
	}
	if dsn := os.Getenv("TEST_AZURE_TIERS_POSTGRES_DSN"); dsn != "" {
		engines["postgres"] = postgres.Open(dsn)
	}
	for name, dialect := range engines {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			old := model.DB
			model.DB = db
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { model.DB = old; _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Option{}))
			require.NoError(t, db.Where(&model.Option{Key: model.DispatchAzureTiersOption}).Delete(&model.Option{}).Error)
			table, err := model.GetAzureTierTable()
			require.NoError(t, err)
			table.Rows = []model.AzureTierRow{{Model: "custom", TPM: []int64{100, 200, 0, 400, 400, 600}}}
			_, err = model.SaveAzureTierTable(table)
			require.NoError(t, err)
			legacy := `{"model":"other","tier":5,"tpm":500,"probed_at":10}`
			ch := model.Channel{Type: 3, Name: "manual-tier-test", Key: "test-only", Models: "custom,other", AzureMeta: &legacy}
			require.NoError(t, db.Create(&ch).Error)
			t.Cleanup(func() { db.Delete(&ch) })
			h := map[string]string{"x-ratelimit-limit-tokens": "400", "x-ratelimit-limit-requests": "25", "x-ms-region": "East US 2"}
			require.NoError(t, model.RecordAzureTestMetadata(&ch, "custom", h, 100))
			var got model.Channel
			require.NoError(t, db.First(&got, ch.Id).Error)
			var entries []map[string]any
			entries = nil
			require.NoError(t, common.UnmarshalJsonStr(*got.AzureMeta, &entries))
			require.Len(t, entries, 2)
			assert.EqualValues(t, 5, entries[0]["tier"])
			assert.EqualValues(t, 4, entries[1]["tier"])
			assert.Equal(t, "eastus2", entries[1]["region"])
			assert.EqualValues(t, 25, entries[1]["rpm"])
			original := *got.AzureMeta
			for _, invalid := range []string{"", "invalid", "0", "-1", "99999999999999999999999"} {
				require.NoError(t, model.RecordAzureTestMetadata(&ch, "custom", map[string]string{"x-ratelimit-limit-tokens": invalid}, 200))
				require.NoError(t, db.First(&got, ch.Id).Error)
				assert.Equal(t, original, *got.AzureMeta)
			}
			h["x-ratelimit-limit-tokens"] = "600"
			require.NoError(t, model.RecordAzureTestMetadata(&ch, "custom", h, 99))
			require.NoError(t, db.First(&got, ch.Id).Error)
			assert.Equal(t, original, *got.AzureMeta)
			h["azureai-fe-offer-type"] = " PTUM "
			require.NoError(t, model.RecordAzureTestMetadata(&ch, "custom", h, 101))
			require.NoError(t, db.First(&got, ch.Id).Error)
			entries = nil
			require.NoError(t, common.UnmarshalJsonStr(*got.AzureMeta, &entries))
			require.Len(t, entries, 2)
			assert.NotContains(t, entries[1], "tier")
			assert.Equal(t, "ptum", entries[1]["offer"])
			delete(h, "azureai-fe-offer-type")
			require.NoError(t, model.RecordAzureTestMetadata(&ch, "unknown", h, 102))
			require.NoError(t, db.First(&got, ch.Id).Error)
			entries = nil
			require.NoError(t, common.UnmarshalJsonStr(*got.AzureMeta, &entries))
			require.Len(t, entries, 3)
			assert.NotContains(t, entries[2], "tier")
			mapping := `{"alias":"custom"}`
			ch.ModelMapping = &mapping
			require.NoError(t, model.RecordAzureTestMetadata(&ch, "alias", h, 103))
			require.NoError(t, db.First(&got, ch.Id).Error)
			entries = nil
			require.NoError(t, common.UnmarshalJsonStr(*got.AzureMeta, &entries))
			require.Len(t, entries, 4)
			assert.EqualValues(t, 6, entries[3]["tier"])
			// A fresh channel starts with NULL metadata. Consecutive model tests merge.
			empty := model.Channel{Type: 3, Name: "fresh-manual-tier", Key: "test-only"}
			require.NoError(t, db.Create(&empty).Error)
			t.Cleanup(func() { db.Delete(&empty) })
			require.NoError(t, model.RecordAzureTestMetadata(&empty, "custom", h, 104))
			require.NoError(t, model.RecordAzureTestMetadata(&empty, "other", h, 105))
			var fresh model.Channel
			require.NoError(t, db.First(&fresh, empty.Id).Error)
			entries = nil
			require.NoError(t, common.UnmarshalJsonStr(*fresh.AzureMeta, &entries))
			require.Len(t, entries, 2)
		})
	}
}

func TestAzureMetadataOnlyChangesOnSuccessfulManualTest(t *testing.T) {
	initModelListColumnNames(t)
	require.NoError(t, i18n.Init())
	oldRatios := ratio_setting.ModelRatio2JSONString()
	oldGroups := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-6.1-sol":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
	})
	setupTaskPluginBindChannelTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	service.InitHttpClient()
	user := model.User{Id: 9881, Username: "azure-manual-test", Group: "default", Status: 1, Quota: 10000000}
	require.NoError(t, model.DB.Create(&user).Error)
	var calls atomic.Int32
	var mode atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if mode.Load() != 2 {
			w.Header().Set("x-ratelimit-limit-tokens", "2500000")
			w.Header().Set("x-ms-region", "East US 2")
		}
		if mode.Load() == 1 {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":{"message":"test failure"}}`)
			return
		}
		fmt.Fprint(w, `{"id":"test","object":"chat.completion","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	oldMeta := `[{"model":"gpt-6.1-sol","tier":1,"tpm":250000,"probed_at":1}]`
	channel := model.Channel{Type: 3, Name: "manual-only", BaseURL: &upstream.URL, Key: "test-only", Models: "gpt-6.1-sol", Group: "default", Status: 1, AzureMeta: &oldMeta}
	require.NoError(t, model.DB.Create(&channel).Error)
	// Automatic health checks use the same upstream response but must not save Tier.
	result := testChannel(context.Background(), &channel, user.Id, "gpt-6.1-sol", string(constant.EndpointTypeOpenAI), false)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	var saved model.Channel
	require.NoError(t, model.DB.First(&saved, channel.Id).Error)
	require.Equal(t, oldMeta, *saved.AzureMeta)
	for _, state := range []int32{0, 1, 2} {
		mode.Store(state)
		before := calls.Load()
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Set("id", user.Id)
		ctx.Set("role", common.RoleRootUser)
		ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(channel.Id)}}
		ctx.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/channel/test/%d?model=gpt-6.1-sol&endpoint_type=openai", channel.Id), nil)
		TestChannel(ctx)
		assert.Equal(t, before+1, calls.Load(), rec.Body.String())
		if state == 1 {
			assert.Contains(t, rec.Body.String(), `"success":false`)
		} else {
			require.Contains(t, rec.Body.String(), `"success":true`)
		}
		require.NoError(t, model.DB.First(&saved, channel.Id).Error)
		if state == 0 {
			var entries []map[string]any
			require.NoError(t, common.UnmarshalJsonStr(*saved.AzureMeta, &entries))
			require.Len(t, entries, 1)
			assert.EqualValues(t, 3, entries[0]["tier"])
			oldMeta = *saved.AzureMeta
		} else {
			assert.Equal(t, oldMeta, *saved.AzureMeta)
		}
	}
	// Response-time updates are asynchronous; drain them before fixture restoration.
	require.Eventually(t, func() bool { var c model.Channel; return model.DB.First(&c, channel.Id).Error == nil && c.TestTime > 0 }, time.Second, 10*time.Millisecond)
}
