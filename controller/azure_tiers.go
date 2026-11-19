package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetAzureTierTable(c *gin.Context) {
	table, err := model.GetAzureTierTable()
	if err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to load Azure Tier table"))
		return
	}
	common.ApiSuccess(c, table)
}

func UpdateAzureTierTable(c *gin.Context) {
	var table model.AzureTierTable
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if err := common.DecodeJson(c.Request.Body, &table); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	table, err := model.SaveAzureTierTable(table)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrAzureTiersInvalid):
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage(model.ErrAzureTiersInvalid.Error()))
		case errors.Is(err, model.ErrAzureTiersConflict):
			common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage(model.ErrAzureTiersConflict.Error()))
		default:
			common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to save Azure Tier table"))
		}
		return
	}
	recordManageAudit(c, "azure_tiers.update", map[string]any{"count": len(table.Rows), "revision": table.Revision})
	common.ApiSuccess(c, table)
}
