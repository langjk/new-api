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
		if role >= common.RoleRootUser {
			assert.Contains(t, body, `"quota":300`)
			assert.Contains(t, body, `"recent_quota":15`)
			assert.Contains(t, body, "zero")
		} else if role >= common.RoleCommonUser {
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

func TestChannelUsageTotalsRangeAndDownstreamIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", path)
	require.NoError(t, os.WriteFile(path, []byte(`{"channel_totals":{"updated_at":1600,"rows":[{"user_id":1,"source":"gpt","quota":100,"downstreams":[{"downstream":"noah","quota":110},{"downstream":"__unattributed__","quota":-10}]},{"user_id":2,"source":"gpt","quota":999,"downstreams":[{"downstream":"private-site","quota":999}]}],"users":[{"id":1,"username":"alice"},{"id":2,"username":"bob"}],"history":[{"start":1000,"end":1300,"rows":[{"user_id":1,"source":"gpt","quota":20,"downstreams":[{"downstream":"noah","quota":25},{"downstream":"__unattributed__","quota":-5}]},{"user_id":2,"source":"gpt","quota":30}]},{"start":1300,"end":1600,"rows":[]}]}}`), 0600))
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		for _, tc := range []struct {
			query, status string
			samples       int
		}{
			{"start_timestamp=1000&end_timestamp=1600", "complete", 2},
			{"start_timestamp=1100&end_timestamp=1600", "complete", 1},
			{"start_timestamp=500&end_timestamp=1600", "partial", 2},
			{"start_timestamp=100&end_timestamp=500", "unavailable", 0},
		} {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("id", 1)
			c.Set("role", role)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/data/channel-totals?"+tc.query+"&user_id=2", nil)
			GetChannelUsageTotals(c)
			var body struct {
				Success bool `json:"success"`
				Data    struct {
					Users []channelUsageGroup `json:"users"`
					Range channelUsageRange   `json:"range"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			require.True(t, body.Success, response.Body.String())
			assert.Equal(t, tc.status, body.Data.Range.Status)
			assert.Equal(t, tc.samples, body.Data.Range.Samples)
			for _, user := range body.Data.Users {
				var sum, period int64
				for _, ds := range user.Downstreams {
					sum += ds.Quota
					period += ds.PeriodQuota
				}
				assert.Equal(t, user.Quota, sum)
				assert.Equal(t, user.PeriodQuota, period)
				if tc.samples == 1 || tc.samples == 0 {
					assert.Zero(t, user.PeriodQuota)
				}
			}
			if role < common.RoleRootUser {
				assert.NotContains(t, response.Body.String(), "private-site")
				assert.NotContains(t, response.Body.String(), "bob")
			}
		}
	}
}

func TestChannelUsageTotalsRejectsInvalidRangesAndMarksGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	t.Setenv("DOWNSTREAM_USAGE_SNAPSHOT", path)
	require.NoError(t, os.WriteFile(path, []byte(`{"channel_totals":{"updated_at":1900,"history":[{"start":1000,"end":1300,"rows":[]},{"start":1300,"end":1600,"error":"counter_changed","rows":[]},{"start":1600,"end":1900,"rows":[]}]}}`), 0600))
	for _, query := range []string{"start_timestamp=abc&end_timestamp=1600", "start_timestamp=1000", "end_timestamp=1600", "start_timestamp=1600&end_timestamp=1000", "start_timestamp=1000&end_timestamp=1000", "start_timestamp=1000&end_timestamp=9999999"} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Set("id", 1)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
		GetChannelUsageTotals(c)
		assert.Contains(t, response.Body.String(), `"success":false`)
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/?start_timestamp=1000&end_timestamp=1900", nil)
	GetChannelUsageTotals(c)
	assert.Contains(t, response.Body.String(), `"status":"partial"`)
	assert.Contains(t, response.Body.String(), `"samples":2`)
}

func TestChannelUsageTotalsRejectsNonReconcilingBreakdown(t *testing.T) {
	row := channelUsageGroup{Quota: 100, Downstreams: []channelUsageDownstream{{Downstream: "site", Quota: 99}}}
	assert.False(t, addChannelUsageGroup(&channelUsageGroup{}, row, false))
	amount := int64(9007199254740991)
	assert.False(t, addChannelUsageAmount(&amount, 1))
}
