package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetProxyPool(c *gin.Context) {
	pool, err := model.GetDispatchProxyPool()
	if err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to load proxy pool"))
		return
	}
	common.ApiSuccess(c, pool)
}

func UpdateProxyPool(c *gin.Context) {
	var pool model.DispatchProxyPool
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := common.DecodeJson(c.Request.Body, &pool); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	pool, err := model.SaveDispatchProxyPool(pool)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrProxyPoolInvalid):
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage(model.ErrProxyPoolInvalid.Error()))
		case errors.Is(err, model.ErrProxyPoolConflict):
			common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage(model.ErrProxyPoolConflict.Error()))
		default:
			common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to save proxy pool"))
		}
		return
	}
	recordManageAudit(c, "proxy_pool.update", map[string]any{"count": len(pool.Proxies)})
	common.ApiSuccess(c, pool)
}
