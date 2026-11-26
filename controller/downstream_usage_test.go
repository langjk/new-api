package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownstreamUsageAuthenticatedFilteredAndSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", path)
	require.NoError(t, os.WriteFile(path, []byte(`{"sites":[{"name":"noahgrape","token":"MUST-NOT-LEAK","data":[{"created_at":1000,"model_name":"included","quota":12,"username":"private"},{"created_at":3000,"model_name":"excluded","quota":9}]}]}`), 0600))
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("role", role)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/data/downstreams?start_timestamp=500&end_timestamp=2000", nil)
		GetDownstreamUsage(ctx)
		body := recorder.Body.String()
		assert.NotContains(t, body, "MUST-NOT-LEAK")
		assert.NotContains(t, body, "private")
		assert.NotContains(t, body, "excluded")
		if role >= common.RoleCommonUser {
			assert.Contains(t, body, "included")
		} else {
			assert.NotContains(t, body, "included")
			assert.Contains(t, body, `"success":false`)
		}
	}
}
func TestDownstreamUsageMissingSnapshotIsNotZeroConsumption(t *testing.T) {
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", filepath.Join(t.TempDir(), "missing"))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/data/downstreams?start_timestamp=500&end_timestamp=2000", nil)
	GetDownstreamUsage(ctx)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestModelTotalsAllHistoryFixedWindowAndSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "totals.json")
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", path)
	require.NoError(t, os.WriteFile(path, []byte(`{"sites":[{"name":"site","token":"secret","data":[{"username":"private"}],"model_totals":{"history_models":[{"model_name":"model","quota":120,"key":"secret"}],"history_updated_at":1000,"history_from":10,"recent_models":[{"model_name":"model","quota":5}],"window_start":700,"window_end":1000,"recent_updated_at":1001,"history_error":"","recent_error":""}},{"name":"missing"}]}`), 0600))
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("role", role)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/data/model-totals?start_timestamp=900", nil)
		GetModelUsageTotals(ctx)
		body := recorder.Body.String()
		assert.NotContains(t, body, "secret")
		assert.NotContains(t, body, "private")
		if role >= common.RoleCommonUser {
			assert.Contains(t, body, `"history_from":10`)
			assert.Contains(t, body, `"window_start":700`)
			assert.Contains(t, body, `"quota":120`)
			assert.Contains(t, body, `"model_totals":null`)
		} else {
			assert.Contains(t, body, `"success":false`)
			assert.NotContains(t, body, `"model_name"`)
		}
	}
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", filepath.Join(t.TempDir(), "missing"))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleCommonUser)
	GetModelUsageTotals(ctx)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}
