package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type ChannelRepushRequest struct {
	Downstream      string `json:"downstream"`
	DownstreamGroup string `json:"downstream_group"`
}

// RepushChannelDispatch 登记一次「重推」：把渠道的某个下发落点（下游 + 下游分组）重新推送。
// 只写 dispatch_requests，由外部 dispatcher 消费（ok/dead/failed 落点都会在下游新建一条）。
func RepushChannelDispatch(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	req := ChannelRepushRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	downstream := strings.TrimSpace(req.Downstream)
	downstreamGroup := strings.TrimSpace(req.DownstreamGroup)
	if downstream == "" || len(downstream) > 64 || len(downstreamGroup) > 255 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if _, err := model.GetChannelById(id, false); err != nil {
		common.ApiErrorT(c, "Channel not found")
		return
	}
	if err := model.CreateDispatchRequest(id, downstream, downstreamGroup, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.dispatch_repush", map[string]any{
		"id":               id,
		"downstream":       downstream,
		"downstream_group": downstreamGroup,
	})
	common.ApiSuccess(c, nil)
}
