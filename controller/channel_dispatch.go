package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// RepushChannelDispatch retains the legacy endpoint without creating work.
func RepushChannelDispatch(c *gin.Context) {
	common.ApiErrorT(c, "Change channel groups to start dispatch")
}

func BindChannelDispatch(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var req struct {
		MasterGroup     string `json:"master_group"`
		RemoteChannelId int    `json:"remote_channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	req.MasterGroup = strings.TrimSpace(req.MasterGroup)
	if req.RemoteChannelId <= 0 || req.RemoteChannelId > 2147483647 || len(req.MasterGroup) > 255 || strings.Contains(req.MasterGroup, ",") ||
		!strings.Contains(req.MasterGroup, "-") || !ratio_setting.ContainsGroupRatio(req.MasterGroup) {
		common.ApiErrorT(c, "Select a registered downstream group and a positive channel ID")
		return
	}
	request, err := model.RequestDispatchBinding(id, req.MasterGroup, req.RemoteChannelId, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.dispatch_bind", map[string]any{
		"id": id, "master_group": req.MasterGroup, "remote_channel_id": req.RemoteChannelId,
	})
	common.ApiSuccess(c, request)
}

func GetChannelDispatchBindings(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		common.ApiErrorT(c, "Unauthorized, insufficient privileges")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	requests := make([]model.DispatchBindingRequest, 0)
	if err := model.DB.Where("channel_id = ?", id).Order("id DESC").Find(&requests).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, requests)
}
