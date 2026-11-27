package controller

import (
	"net/http"
	"os"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type channelUsageDownstream struct {
	Downstream  string `json:"downstream"`
	Quota       int64  `json:"quota"`
	RecentQuota int64  `json:"recent_quota"`
	PeriodQuota int64  `json:"period_quota"`
}

type channelUsageGroup struct {
	UserID        int                      `json:"user_id"`
	Username      string                   `json:"username"`
	DisplayName   string                   `json:"display_name"`
	Source        string                   `json:"source"`
	Quota         int64                    `json:"quota"`
	RecentQuota   int64                    `json:"recent_quota"`
	PeriodQuota   int64                    `json:"period_quota"`
	ChannelCount  int64                    `json:"channel_count"`
	RetainedCount int64                    `json:"retained_count"`
	Downstreams   []channelUsageDownstream `json:"downstreams"`
}

type channelUsageUser struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type channelUsageInterval struct {
	Start int64               `json:"start"`
	End   int64               `json:"end"`
	Error string              `json:"error"`
	Rows  []channelUsageGroup `json:"rows"`
}

type channelUsageRange struct {
	Active         bool   `json:"active"`
	RequestedStart int64  `json:"requested_start"`
	RequestedEnd   int64  `json:"requested_end"`
	ActualStart    int64  `json:"actual_start"`
	ActualEnd      int64  `json:"actual_end"`
	AvailableFrom  int64  `json:"available_from"`
	AvailableUntil int64  `json:"available_until"`
	Samples        int    `json:"samples"`
	Status         string `json:"status"`
}

// Snapshot quotas are integers within JavaScript's exact range. Attribution
// corrections may be signed; owner and source consumption must be nonnegative.
func addChannelUsageAmount(target *int64, amount int64) bool {
	const limit int64 = 9007199254740991
	if amount < -limit || amount > limit || *target+amount < -limit || *target+amount > limit {
		return false
	}
	*target += amount
	return true
}

func addChannelUsageGroup(target *channelUsageGroup, row channelUsageGroup, period bool) bool {
	if row.Quota < 0 || row.RecentQuota < 0 {
		return false
	}
	parts := row.Downstreams
	if len(parts) == 0 {
		parts = []channelUsageDownstream{{Downstream: "__unattributed__", Quota: row.Quota, RecentQuota: row.RecentQuota}}
	}
	var total, recent int64
	for _, part := range parts {
		if part.Downstream == "" || !addChannelUsageAmount(&total, part.Quota) || !addChannelUsageAmount(&recent, part.RecentQuota) {
			return false
		}
	}
	if total != row.Quota || recent != row.RecentQuota {
		return false
	}
	if period {
		if !addChannelUsageAmount(&target.PeriodQuota, row.Quota) {
			return false
		}
	} else {
		if !addChannelUsageAmount(&target.Quota, row.Quota) || !addChannelUsageAmount(&target.RecentQuota, row.RecentQuota) {
			return false
		}
		target.ChannelCount += row.ChannelCount
		target.RetainedCount += row.RetainedCount
	}
	for _, part := range parts {
		index := -1
		for i := range target.Downstreams {
			if target.Downstreams[i].Downstream == part.Downstream {
				index = i
				break
			}
		}
		if index < 0 {
			target.Downstreams = append(target.Downstreams, channelUsageDownstream{Downstream: part.Downstream})
			index = len(target.Downstreams) - 1
		}
		dest := &target.Downstreams[index]
		if period {
			if !addChannelUsageAmount(&dest.PeriodQuota, part.Quota) {
				return false
			}
		} else if !addChannelUsageAmount(&dest.Quota, part.Quota) || !addChannelUsageAmount(&dest.RecentQuota, part.RecentQuota) {
			return false
		}
	}
	return true
}

// Owner filtering applies to cumulative and historical rows before aggregation.
// Only whitelisted summaries leave the server, never private channel baselines.
func GetChannelUsageTotals(c *gin.Context) {
	if c.GetInt("role") < common.RoleCommonUser || c.GetInt("id") <= 0 {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	selection := channelUsageRange{Status: "all_time"}
	if c.Query("start_timestamp") != "" || c.Query("end_timestamp") != "" {
		start, end, ok := parseFlowQuotaTimeRange(c)
		if !ok {
			return
		}
		if end <= start || end-start > 30*86400 {
			common.ApiErrorT(c, "Select a valid time range of up to 30 days")
			return
		}
		selection.Active = true
		selection.RequestedStart, selection.RequestedEnd = start, end
		selection.Status = "complete"
	}
	path := os.Getenv("DOWNSTREAM_USAGE_SNAPSHOT")
	if path == "" {
		path = "/data/downstream-usage.json"
	}
	raw, err := os.ReadFile(path)
	var snapshot struct {
		Totals *struct {
			Metric      string                 `json:"metric"`
			UpdatedAt   int64                  `json:"updated_at"`
			WindowStart int64                  `json:"window_start"`
			WindowEnd   int64                  `json:"window_end"`
			Error       string                 `json:"error"`
			RecentError string                 `json:"recent_error"`
			Rows        []channelUsageGroup    `json:"rows"`
			Users       []channelUsageUser     `json:"users"`
			History     []channelUsageInterval `json:"history"`
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
			owners[user.ID] = &channelUsageGroup{UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Downstreams: []channelUsageDownstream{}}
		}
	}
	selected := make([]channelUsageGroup, 0)
	if len(data.History) > 0 {
		selection.AvailableFrom = data.History[0].Start
		selection.AvailableUntil = data.History[len(data.History)-1].End
	}
	if selection.Active {
		if selection.RequestedStart < selection.AvailableFrom || selection.RequestedEnd > selection.AvailableUntil+360 || data.Error != "" {
			selection.Status = "partial"
		}
		var previousEnd int64
		for _, interval := range data.History {
			if interval.End <= selection.RequestedStart || interval.Start >= selection.RequestedEnd {
				continue
			}
			if interval.Error != "" || interval.End-interval.Start < 240 || interval.End-interval.Start > 360 || (previousEnd != 0 && interval.Start != previousEnd) {
				selection.Status = "partial"
				previousEnd = interval.End
				continue
			}
			previousEnd = interval.End
			// Do not prorate cumulative increments into invented event timestamps.
			if interval.Start < selection.RequestedStart || interval.End > selection.RequestedEnd {
				continue
			}
			if selection.Samples == 0 {
				selection.ActualStart = interval.Start
			}
			selection.ActualEnd = interval.End
			selection.Samples++
			selected = append(selected, interval.Rows...)
		}
		if selection.Samples == 0 {
			selection.Status = "unavailable"
		}
	}
	for pass, rows := range [][]channelUsageGroup{data.Rows, selected} {
		for _, row := range rows {
			if !allUsers && row.UserID != c.GetInt("id") {
				continue
			}
			if owners[row.UserID] == nil {
				owners[row.UserID] = &channelUsageGroup{UserID: row.UserID, Username: row.Username, DisplayName: row.DisplayName, Downstreams: []channelUsageDownstream{}}
			}
			if sources[row.Source] == nil {
				sources[row.Source] = &channelUsageGroup{Source: row.Source, Downstreams: []channelUsageDownstream{}}
			}
			for _, target := range []*channelUsageGroup{owners[row.UserID], sources[row.Source]} {
				if !addChannelUsageGroup(target, row, pass == 1) {
					common.ApiErrorT(c, "Downstream statistics are not available yet")
					return
				}
			}
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
		"error": data.Error, "recent_error": data.RecentError, "range": selection,
	}})
}
