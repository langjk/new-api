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
)

// PreflightChannel tests fresh credentials and applies the live error rules,
// independently of relay auto-disable and the response time threshold.
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
	testErr := result.newAPIError
	shouldDisable := service.MatchesChannelDisableRules(testErr)
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
