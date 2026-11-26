package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"sort"
)

type channelUsageGroup struct {
	UserID        int    `json:"user_id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Source        string `json:"source"`
	Quota         int64  `json:"quota"`
	RecentQuota   int64  `json:"recent_quota"`
	ChannelCount  int64  `json:"channel_count"`
	RetainedCount int64  `json:"retained_count"`
}
type channelUsageUser struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// Owner filtering happens before aggregation; private per-channel baselines are omitted.
func GetChannelUsageTotals(c *gin.Context) {
	if c.GetInt("role") < common.RoleCommonUser || c.GetInt("id") <= 0 {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	path := os.Getenv("DOWNSTREAM_USAGE_SNAPSHOT")
	if path == "" {
		path = "/data/downstream-usage.json"
	}
	raw, err := os.ReadFile(path)
	var snapshot struct {
		Totals *struct {
			Metric      string              `json:"metric"`
			UpdatedAt   int64               `json:"updated_at"`
			WindowStart int64               `json:"window_start"`
			WindowEnd   int64               `json:"window_end"`
			Error       string              `json:"error"`
			RecentError string              `json:"recent_error"`
			Rows        []channelUsageGroup `json:"rows"`
			Users       []channelUsageUser  `json:"users"`
		} `json:"channel_totals"`
	}
	if err != nil || common.Unmarshal(raw, &snapshot) != nil || snapshot.Totals == nil || snapshot.Totals.UpdatedAt == 0 {
		common.ApiErrorT(c, "Downstream statistics are not available yet")
		return
	}
	data := snapshot.Totals
	allUsers := c.GetInt("role") >= common.RoleRootUser
	owners := map[int]*channelUsageGroup{}
	sources := map[string]*channelUsageGroup{}
	for _, user := range data.Users {
		if allUsers || user.ID == c.GetInt("id") {
			owners[user.ID] = &channelUsageGroup{UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName}
		}
	}
	for _, row := range data.Rows {
		if !allUsers && row.UserID != c.GetInt("id") {
			continue
		}
		if row.Quota < 0 || row.RecentQuota < 0 {
			common.ApiErrorT(c, "Downstream statistics are not available yet")
			return
		}
		if owners[row.UserID] == nil {
			owners[row.UserID] = &channelUsageGroup{UserID: row.UserID, Username: row.Username, DisplayName: row.DisplayName}
		}
		if sources[row.Source] == nil {
			sources[row.Source] = &channelUsageGroup{Source: row.Source}
		}
		for _, target := range []*channelUsageGroup{owners[row.UserID], sources[row.Source]} {
			if row.Quota > 9007199254740991-target.Quota || row.RecentQuota > 9007199254740991-target.RecentQuota {
				common.ApiErrorT(c, "Downstream statistics are not available yet")
				return
			}
			target.Quota += row.Quota
			target.RecentQuota += row.RecentQuota
			target.ChannelCount += row.ChannelCount
			target.RetainedCount += row.RetainedCount
		}
	}
	userRows := make([]channelUsageGroup, 0, len(owners))
	sourceRows := make([]channelUsageGroup, 0, len(sources))
	for _, row := range owners {
		userRows = append(userRows, *row)
	}
	for _, row := range sources {
		sourceRows = append(sourceRows, *row)
	}
	sort.Slice(userRows, func(i, j int) bool {
		if userRows[i].Quota == userRows[j].Quota {
			return userRows[i].UserID < userRows[j].UserID
		}
		return userRows[i].Quota > userRows[j].Quota
	})
	sort.Slice(sourceRows, func(i, j int) bool { return sourceRows[i].Source < sourceRows[j].Source })
	c.JSON(http.StatusOK, gin.H{"success": true, "quota_per_unit": common.QuotaPerUnit, "data": gin.H{
		"sources": sourceRows, "users": userRows, "all_users": allUsers, "metric": data.Metric,
		"updated_at": data.UpdatedAt, "window_start": data.WindowStart, "window_end": data.WindowEnd,
		"error": data.Error, "recent_error": data.RecentError,
	}})
}
