package controller

import (
	"encoding/base64"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

type preflightDiagnostic struct {
	Message        string `json:"message,omitempty"`
	ErrorSource    string `json:"error_source,omitempty"`
	UpstreamStatus int    `json:"upstream_status,omitempty"`
	Model          string `json:"model,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
}

// Preflight errors reach channel owners through dispatch_info. Keep the upstream
// response, but redact credentials even if the provider echoes them in its body.
func redactPreflightError(text string, channel *model.Channel, usingKey string) string {
	secrets := []string{channel.Key, usingKey}
	secrets = append(secrets, channel.GetKeys()...)
	for _, key := range slices.Clone(secrets) {
		secrets = append(secrets, strings.FieldsFunc(key, func(r rune) bool { return r == '|' || r == '\n' })...)
		var credentials any
		if common.UnmarshalJsonStr(key, &credentials) == nil {
			pending := []any{credentials}
			for len(pending) > 0 {
				value := pending[0]
				pending = pending[1:]
				switch v := value.(type) {
				case string:
					secrets = append(secrets, v)
				case map[string]any:
					for _, item := range v {
						pending = append(pending, item)
					}
				case []any:
					pending = append(pending, v...)
				}
			}
		}
	}
	for _, value := range channel.GetHeaderOverride() {
		if value, ok := value.(string); ok {
			secrets = append(secrets, value)
			if scheme, token, ok := strings.Cut(value, " "); ok && (strings.EqualFold(scheme, "Bearer") || strings.EqualFold(scheme, "Basic")) {
				secrets = append(secrets, token)
			}
		}
	}
	var setting struct {
		Proxy string `json:"proxy"`
	}
	if channel.Setting != nil {
		_ = common.UnmarshalJsonStr(*channel.Setting, &setting)
	}
	for _, address := range []string{channel.GetBaseURL(), setting.Proxy} {
		if u, err := url.Parse(address); err == nil {
			if u.User != nil {
				password, _ := u.User.Password()
				secrets = append(secrets, u.User.String(), u.User.Username(), password,
					base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+password)))
			}
			for _, values := range u.Query() {
				secrets = append(secrets, values...)
			}
		}
	}
	// Replace longer and encoded values first so partial masking cannot expose a suffix.
	for _, secret := range slices.Clone(secrets) {
		if secret == "" {
			continue
		}
		encoded, _ := common.Marshal(secret)
		if len(encoded) >= 2 {
			secrets = append(secrets, string(encoded[1:len(encoded)-1]))
		}
		secrets = append(secrets, url.QueryEscape(secret), url.PathEscape(secret))
	}
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	replacements := make([]string, 0, len(secrets)*2)
	for _, secret := range secrets {
		if secret != "" {
			replacements = append(replacements, secret, "[redacted]")
		}
	}
	text = strings.NewReplacer(replacements...).Replace(text)
	return common.MaskSensitiveInfo(text, channel.GetBaseURL())
}

func channelPreflightDiagnostic(channel *model.Channel, result testResult) preflightDiagnostic {
	diagnostic := preflightDiagnostic{}
	if result.newAPIError != nil {
		diagnostic.Message = result.newAPIError.Error()
	} else if result.localErr != nil {
		diagnostic.Message = result.localErr.Error()
	} else {
		return diagnostic
	}
	diagnostic.ErrorSource = "local"
	if result.upstreamStatus != 0 {
		diagnostic.ErrorSource = "upstream"
		diagnostic.UpstreamStatus = result.upstreamStatus
	}
	usingKey := ""
	if result.context != nil {
		usingKey = common.GetContextKeyString(result.context, constant.ContextKeyChannelKey)
		diagnostic.Model = common.GetContextKeyString(result.context, constant.ContextKeyOriginalModel)
		if result.context.Request != nil && result.context.Request.URL != nil {
			diagnostic.Endpoint = result.context.Request.URL.Path
		}
	}
	diagnostic.Message = redactPreflightError(diagnostic.Message, channel, usingKey)
	diagnostic.Model = redactPreflightError(diagnostic.Model, channel, usingKey)
	diagnostic.Endpoint = redactPreflightError(diagnostic.Endpoint, channel, usingKey)
	return diagnostic
}

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
		service.DisableChannel(*types.NewChannelError(current.Id, current.Type, current.Name, current.ChannelInfo.IsMultiKey, usingKey, current.GetAutoBan()), redactPreflightError(testErr.MaskSensitiveErrorWithStatusCode(current.GetBaseURL()), current, usingKey))
		confirmed, readErr := model.GetChannelById(id, false)
		if readErr == nil && confirmed.Status == common.ChannelStatusAutoDisabled {
			model.InitChannelCache()
			outcome = "dead"
		}
	} else if result.localErr == nil && result.newAPIError == nil {
		outcome = "ok"
	} else {
		// A completed probe is advisory unless the live disable policy blocks it.
		// Keep this distinct from a healthy result, including ordinary 429s.
		outcome = "allowed"
	}
	recordManageAudit(c, "channel.dispatch_preflight", map[string]any{"id": id, "outcome": outcome})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": struct {
		Outcome string `json:"outcome"`
		preflightDiagnostic
	}{outcome, channelPreflightDiagnostic(channel, result)}})
}
