package controller

import (
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type channelAzureQuota struct {
	Regions []string
	Tier    int
	PTU     bool
}

// Read stored probe results only. A mixed channel uses its lowest non-PTU
// tier, matching AzureQuotaBadge; PTU remains independently selectable.
func readChannelAzureQuota(meta *string) channelAzureQuota {
	var quota channelAzureQuota
	if meta == nil || strings.TrimSpace(*meta) == "" {
		return quota
	}
	var decoded any
	if common.UnmarshalJsonStr(*meta, &decoded) != nil {
		return quota
	}
	entries, ok := decoded.([]any)
	if !ok {
		entries = []any{decoded}
	}
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		region, _ := entry["region"].(string)
		region = strings.ToLower(strings.TrimSpace(region))
		if region != "" && !slices.Contains(quota.Regions, region) {
			quota.Regions = append(quota.Regions, region)
		}
		offer, _ := entry["offer"].(string)
		if strings.EqualFold(strings.TrimSpace(offer), "ptum") {
			quota.PTU = true
			continue
		}
		tier, _ := entry["tier"].(float64)
		if tier >= 1 && tier <= 6 && tier == float64(int(tier)) && (quota.Tier == 0 || int(tier) < quota.Tier) {
			quota.Tier = int(tier)
		}
	}
	return quota
}

func channelAzureRegions(channels []*model.Channel) []string {
	regions := make([]string, 0)
	for _, channel := range channels {
		regions = append(regions, readChannelAzureQuota(channel.AzureMeta).Regions...)
	}
	slices.Sort(regions)
	return slices.Compact(regions)
}

func channelMatchesAzureQuota(channel *model.Channel, region, tier string) bool {
	quota := readChannelAzureQuota(channel.AzureMeta)
	if region != "" && !slices.Contains(quota.Regions, region) {
		return false
	}
	if tier == "ptu" {
		return quota.PTU
	}
	return tier == "" || (quota.Tier > 0 && strconv.Itoa(quota.Tier) == tier)
}
