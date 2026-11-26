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

func TestChannelUsageTotalsOwnerIsolationAndSanitization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", path)
	require.NoError(t, os.WriteFile(path, []byte(`{"channel_totals":{"metric":"channel_sync_delta","updated_at":1000,"window_start":700,"window_end":1000,"channels":{"private-key":"hidden"},"rows":[{"user_id":1,"username":"alice","source":"gpt-openai","quota":100,"recent_quota":5,"channel_count":1},{"user_id":2,"username":"bob","source":"gpt-openai","quota":200,"recent_quota":10,"channel_count":2}],"users":[{"id":1,"username":"alice"},{"id":2,"username":"bob"},{"id":3,"username":"zero"}]}}`), 0600))
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Set("id", 1)
		c.Set("role", role)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/data/channel-totals?user_id=2", nil)
		GetChannelUsageTotals(c)
		body := response.Body.String()
		assert.NotContains(t, body, "private-key")
		assert.NotContains(t, body, "hidden")
		if role >= common.RoleAdminUser {
			assert.Contains(t, body, `"quota":300`)
			assert.Contains(t, body, `"recent_quota":15`)
			assert.Contains(t, body, "zero")
		} else if role == common.RoleCommonUser {
			assert.Contains(t, body, `"quota":100`)
			assert.NotContains(t, body, "bob")
			assert.NotContains(t, body, "zero")
		} else {
			assert.Contains(t, body, `"success":false`)
			assert.NotContains(t, body, "alice")
		}
	}
}

func TestChannelUsageTotalsMissingSnapshotIsUnavailable(t *testing.T) {
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", filepath.Join(t.TempDir(), "missing"))
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set("id", 1)
	c.Set("role", common.RoleCommonUser)
	GetChannelUsageTotals(c)
	assert.Contains(t, r.Body.String(), `"success":false`)
}
