package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"reflect"
	"strconv"
	"time"
)

// PreflightChannel tests fresh credentials and applies the same live auto-disable
// policy as scheduled health checks. Only the server changes channel status.
func PreflightChannel(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiErrorT(c, "Channel does not exist")
		return
	}
	outcome := "unknown"
	if channel.Status != common.ChannelStatusEnabled {
		common.ApiSuccess(c, gin.H{"outcome": outcome})
		return
	}
	started := time.Now()
	result := testChannel(c.Request.Context(), channel, c.GetInt("id"), "", "", shouldUseStreamForAutomaticChannelTest(channel))
	if c.Request.Context().Err() != nil {
		return
	}
	current, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiErrorT(c, "Channel does not exist")
		return
	}
	// Never apply an old test result to credentials edited while the test ran.
	originalConfig := []any{channel.Type, channel.Key, channel.BaseURL, channel.Models, channel.TestModel, channel.Setting, channel.OtherSettings, channel.Other, channel.ModelMapping, channel.ParamOverride, channel.HeaderOverride}
	currentConfig := []any{current.Type, current.Key, current.BaseURL, current.Models, current.TestModel, current.Setting, current.OtherSettings, current.Other, current.ModelMapping, current.ParamOverride, current.HeaderOverride}
	if !reflect.DeepEqual(originalConfig, currentConfig) || current.Status != common.ChannelStatusEnabled {
		common.ApiSuccess(c, gin.H{"outcome": outcome})
		return
	}
	threshold := int64(common.ChannelDisableThreshold * 1000)
	if threshold == 0 {
		threshold = 10000000
	}
	shouldDisable, testErr := channelTestDisableDecision(result, time.Since(started).Milliseconds(), threshold)
	if shouldDisable && current.GetAutoBan() {
		usingKey := ""
		if result.context != nil {
			usingKey = common.GetContextKeyString(result.context, constant.ContextKeyChannelKey)
		}
		service.DisableChannel(*types.NewChannelError(current.Id, current.Type, current.Name, current.ChannelInfo.IsMultiKey, usingKey, current.GetAutoBan()), testErr.MaskSensitiveErrorWithStatusCode(current.GetBaseURL()))
		confirmed, readErr := model.GetChannelById(id, false)
		if readErr == nil && confirmed.Status == common.ChannelStatusAutoDisabled {
			model.InitChannelCache()
			outcome = "dead"
		}
	} else if result.localErr == nil && result.newAPIError == nil {
		outcome = "ok"
	}
	recordManageAudit(c, "channel.dispatch_preflight", map[string]any{"id": id, "outcome": outcome})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"outcome": outcome}})
}
