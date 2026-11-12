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

func TestDownstreamUsageRootOnlyFilteredAndSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", path)
	require.NoError(t, os.WriteFile(path, []byte(`{"sites":[{"name":"noahgrape","token":"MUST-NOT-LEAK","data":[{"created_at":1000,"model_name":"included","quota":12,"username":"private"},{"created_at":3000,"model_name":"excluded","quota":9}]}]}`), 0600))
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("role", role)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/data/downstreams?start_timestamp=500&end_timestamp=2000", nil)
		GetDownstreamUsage(ctx)
		body := recorder.Body.String()
		assert.NotContains(t, body, "MUST-NOT-LEAK")
		assert.NotContains(t, body, "private")
		assert.NotContains(t, body, "excluded")
		if role == common.RoleRootUser {
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
