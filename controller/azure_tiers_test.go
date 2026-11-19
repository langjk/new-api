package controller

import (
	"net/http"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
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
