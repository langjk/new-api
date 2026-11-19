package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestProxyPoolProbeValidationAndFailure(t *testing.T) {
	router := gin.New()
	router.POST("/test", TestProxyPoolProxy)
	for _, body := range []string{`{}`, `{"proxy":""}`, `{"proxy":"ftp://user:secret@host"}`, `{"proxy":"http://host/path"}`} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.NotContains(t, w.Body.String(), "secret")
	}
	// A rejected CONNECT must never fall back to direct access or echo credentials.
	called := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- r.Method + " " + r.Host
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxy.Close()
	body, err := common.Marshal(map[string]string{"proxy": strings.Replace(proxy.URL, "://", "://user:secret@", 1)})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "CONNECT www.google.com:443", <-called)
	assert.Contains(t, w.Body.String(), `"reachable":false`)
	assert.Contains(t, w.Body.String(), "Proxy connection failed")
	assert.NotContains(t, w.Body.String(), "secret")
}

func TestProxyPoolStorage(t *testing.T) {
	engines := map[string]gorm.Dialector{"sqlite": sqlite.Open(":memory:")}
	if dsn := os.Getenv("TEST_PROXYPOOL_MYSQL_DSN"); dsn != "" {
		engines["mysql"] = mysql.Open(dsn)
	}
	if dsn := os.Getenv("TEST_PROXYPOOL_POSTGRES_DSN"); dsn != "" {
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
			require.NoError(t, db.Where(&model.Option{Key: model.DispatchProxyPoolOption}).Delete(&model.Option{}).Error)
			empty, err := model.GetDispatchProxyPool()
			require.NoError(t, err)
			require.Empty(t, empty.Proxies)
			pool, err := model.SaveDispatchProxyPool(model.DispatchProxyPool{Proxies: []model.DispatchProxy{
				{Name: "one", Proxy: " socks5://user:secret@HOST ", Enabled: true},
				{Name: "two", Proxy: "https://host:8080", Enabled: false},
			}})
			require.NoError(t, err)
			require.NotEmpty(t, pool.Revision)
			assert.Equal(t, "socks5://user:secret@host:1080", pool.Proxies[0].Proxy)
			require.NotEmpty(t, pool.Proxies[0].ID)
			// Existing option data survives repeated startup/schema checks.
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.Option{}))
				got, err := model.GetDispatchProxyPool()
				require.NoError(t, err)
				assert.Equal(t, pool, got)
			}
			_, err = model.SaveDispatchProxyPool(empty)
			require.ErrorIs(t, err, model.ErrProxyPoolConflict)
			for _, proxy := range []string{"", "ftp://host", "http://host/path", "socks5://host:99999", "http://host?q=secret"} {
				_, err = model.SaveDispatchProxyPool(model.DispatchProxyPool{Revision: pool.Revision, Proxies: []model.DispatchProxy{{Name: "bad", Proxy: proxy}}})
				require.ErrorIs(t, err, model.ErrProxyPoolInvalid)
				assert.NotContains(t, err.Error(), "secret")
			}
			_, err = model.SaveDispatchProxyPool(model.DispatchProxyPool{Revision: pool.Revision, Proxies: []model.DispatchProxy{pool.Proxies[0], pool.Proxies[0]}})
			require.ErrorIs(t, err, model.ErrProxyPoolInvalid)
			cleared, err := model.SaveDispatchProxyPool(model.DispatchProxyPool{Revision: pool.Revision, Proxies: []model.DispatchProxy{}})
			require.NoError(t, err)
			require.Empty(t, cleared.Proxies)
			_, err = model.SaveDispatchProxyPool(pool)
			require.ErrorIs(t, err, model.ErrProxyPoolConflict)
		})
	}
}

func TestProxyPoolRootAccessAndSecrets(t *testing.T) {
	user, token := setupAccessTokenAudit(t)
	router := gin.New()
	router.GET("/api/proxy-pool/", middleware.RootAuth(), middleware.DisableCache(), GetProxyPool)
	router.PUT("/api/proxy-pool/", middleware.RootAuth(), middleware.DisableCache(), UpdateProxyPool)
	router.POST("/api/proxy-pool/test", middleware.RootAuth(), middleware.DisableCache(), TestProxyPoolProxy)
	router.GET("/api/option/", middleware.RootAuth(), GetOptions)
	router.PUT("/api/option/", middleware.RootAuth(), UpdateOption)
	body := `{"revision":"","proxies":[{"id":"","name":"test","proxy":"socks5://user:pool-secret@host:1080","enabled":true}]}`
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		require.NoError(t, model.DB.Model(user).Update("role", role).Error)
		probe := accessTokenRequest(router, http.MethodPost, "/api/proxy-pool/test", token, "", `{"proxy":""}`)
		if role == common.RoleRootUser {
			assert.Equal(t, http.StatusBadRequest, probe.Code)
		} else {
			assert.Equal(t, http.StatusForbidden, probe.Code)
		}
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			response := accessTokenRequest(router, method, "/api/proxy-pool/", token, "", body)
			if role == common.RoleRootUser {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
			} else {
				assert.Equal(t, http.StatusForbidden, response.Code)
				assert.NotContains(t, response.Body.String(), "pool-secret")
			}
		}
	}
	readOnly, _ := createScopedAccessToken(t, user.Id, -1, "option:read")
	assert.Equal(t, http.StatusForbidden, accessTokenRequest(router, http.MethodPost, "/api/proxy-pool/test", readOnly, "", `{}`).Code)
	assert.Equal(t, http.StatusUnauthorized, accessTokenRequest(router, http.MethodPost, "/api/proxy-pool/test", "", "", `{}`).Code)
	response := accessTokenRequest(router, http.MethodGet, "/api/proxy-pool/", readOnly, "", "")
	assert.Equal(t, http.StatusForbidden, response.Code)
	response = accessTokenRequest(router, http.MethodGet, "/api/proxy-pool/", "", "", "")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	response = accessTokenRequest(router, http.MethodPut, "/api/option/", token, "", `{"key":"DispatchProxyPool","value":"bad"}`)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	previous := common.OptionMap
	common.OptionMap = map[string]string{model.DispatchProxyPoolOption: body}
	t.Cleanup(func() { common.OptionMap = previous })
	response = accessTokenRequest(router, http.MethodGet, "/api/option/", token, "", "")
	assert.NotContains(t, response.Body.String(), "pool-secret")
	var logs []model.AuditLog
	require.NoError(t, model.DB.Find(&logs).Error)
	encoded, err := common.Marshal(logs)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "pool-secret")
}
