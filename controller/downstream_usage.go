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
