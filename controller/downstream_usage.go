package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

type downstreamUsagePoint struct {
	CreatedAt int64  `json:"created_at"`
	ModelName string `json:"model_name"`
	Quota     int64  `json:"quota"`
	TokenUsed int64  `json:"token_used"`
	Count     int64  `json:"count"`
}
type downstreamUsageSite struct {
	Name          string                 `json:"name"`
	UpdatedAt     int64                  `json:"updated_at"`
	AvailableFrom int64                  `json:"available_from"`
	Error         string                 `json:"error"`
	Data          []downstreamUsagePoint `json:"data"`
}

type modelUsageTotal struct {
	ModelName string `json:"model_name"`
	Quota     int64  `json:"quota"`
}

type modelUsageSnapshot struct {
	Metric              string            `json:"metric"`
	HistoryModels       []modelUsageTotal `json:"history_models"`
	HistoryUnattributed int64             `json:"history_unattributed"`
	HistoryFrom         int64             `json:"history_from"`
	HistoryUpdatedAt    int64             `json:"history_updated_at"`
	HistoryError        string            `json:"history_error"`
	RecentModels        []modelUsageTotal `json:"recent_models"`
	RecentUnattributed  int64             `json:"recent_unattributed"`
	WindowStart         int64             `json:"window_start"`
	WindowEnd           int64             `json:"window_end"`
	RecentUpdatedAt     int64             `json:"recent_updated_at"`
	RecentError         string            `json:"recent_error"`
}

// GetModelUsageTotals exposes the same downstream scope as the existing charts.
// Only explicit aggregate fields cross the server/browser boundary.
func GetModelUsageTotals(c *gin.Context) {
	if c.GetInt("role") < common.RoleCommonUser {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	path := os.Getenv("DOWNSTREAM_USAGE_SNAPSHOT")
	if path == "" {
		path = "/data/downstream-usage.json"
	}
	data, err := os.ReadFile(path)
	var snapshot struct {
		Sites []struct {
			Name   string              `json:"name"`
			Totals *modelUsageSnapshot `json:"model_totals"`
		} `json:"sites"`
	}
	if err != nil || common.Unmarshal(data, &snapshot) != nil {
		common.ApiErrorT(c, "Downstream statistics are not available yet")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snapshot.Sites, "quota_per_unit": common.QuotaPerUnit})
}

// GetDownstreamUsage serves only sanitized local snapshots. No remote credentials
// or browser-controlled downstream URLs are involved in this request.
func GetDownstreamUsage(c *gin.Context) {
	if c.GetInt("role") < common.RoleCommonUser {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	start, end, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	path := os.Getenv("DOWNSTREAM_USAGE_SNAPSHOT")
	if path == "" {
		path = "/data/downstream-usage.json"
	}
	data, err := os.ReadFile(path)
	var snapshot struct {
		Sites []downstreamUsageSite `json:"sites"`
	}
	if err != nil || common.Unmarshal(data, &snapshot) != nil {
		common.ApiErrorT(c, "Downstream statistics are not available yet")
		return
	}
	for i := range snapshot.Sites {
		filtered := make([]downstreamUsagePoint, 0)
		for _, row := range snapshot.Sites[i].Data {
			if row.CreatedAt >= start && row.CreatedAt <= end {
				filtered = append(filtered, row)
			}
		}
		snapshot.Sites[i].Data = filtered
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snapshot.Sites})
}
