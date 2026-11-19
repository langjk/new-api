package controller

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// TestProxyPoolProxy probes only a fixed HTTPS target, through the supplied proxy.
// Never return transport errors: they can contain proxy credentials.
func TestProxyPoolProxy(c *gin.Context) {
	var input struct {
		Proxy string `json:"proxy"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := common.DecodeJson(c.Request.Body, &input); err != nil || len(input.Proxy) > 2048 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid proxy URL"))
		return
	}
	proxyURL, err := common.ParseProxyURLStrict(input.Proxy)
	if err != nil || proxyURL == nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid proxy URL"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.google.com/generate_204", nil)
	start := time.Now()
	resp, err := client.Do(req)
	result := gin.H{"reachable": false, "latency_ms": time.Since(start).Milliseconds(), "status_code": 0, "message": "Proxy connection failed"}
	if err != nil {
		var netErr net.Error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			result["message"] = "Proxy test timed out"
		}
	} else {
		resp.Body.Close()
		result["status_code"] = resp.StatusCode
		result["reachable"] = resp.StatusCode == http.StatusNoContent
		result["message"] = "Proxy test returned unexpected HTTP status"
		if resp.StatusCode == http.StatusNoContent {
			result["message"] = "Proxy connection successful"
		}
	}
	common.ApiSuccess(c, result)
}

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
