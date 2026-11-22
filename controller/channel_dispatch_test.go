package controller

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestManualDispatchBindingRequiresRoot(t *testing.T) {
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
		for _, handler := range []gin.HandlerFunc{BindChannelDispatch, GetChannelDispatchBindings} {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("role", role)
			handler(ctx)
			require.Contains(t, recorder.Body.String(), `"success":false`)
		}
	}
}

func TestManualDispatchDatabase(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldMaster, oldPath, oldRedis := common.IsMasterNode, common.SQLitePath, common.RedisEnabled
	oldGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.IsMasterNode, common.SQLitePath, common.RedisEnabled = oldMaster, oldPath, oldRedis
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = filepath.Join(t.TempDir(), "manual-dispatch.db")
	require.NoError(t, model.InitDB())
	db := model.DB
	model.LOG_DB = db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	query := "SELECT VERSION()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		query = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(query).Scan(&version).Error)
	t.Logf("database=%s version=%s", common.MainDatabaseType(), version)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.DispatchRequest{}, &model.User{}, &model.Log{}, &model.AuditLog{}, &model.CasbinRule{}, &model.AuthzRole{}))
	require.NoError(t, authz.Init(db))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"noahgrape-azure":1}`))
	legacy := model.Channel{Type: 14, Name: "legacy", Key: "private-key", Group: "noahgrape-azure", Models: "claude-test", DispatchInfo: `[{"rid":777}]`, CreatorId: 2}
	require.NoError(t, db.Create(&legacy).Error)
	// Upgrade from the deployed schema twice: preserve legacy channel routing and IDs.
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.DispatchBindingRequest{}))
	}
	var saved model.Channel
	require.NoError(t, db.First(&saved, legacy.Id).Error)
	require.Equal(t, legacy.Group, saved.Group)
	require.Equal(t, legacy.DispatchInfo, saved.DispatchInfo)
	for _, mode := range []string{"single", "batch"} {
		body := fmt.Sprintf(`{"mode":%q,"channel":{"type":1,"name":"new","key":"test-key","group":"noahgrape-azure","models":"gpt"}}`, mode)
		response := postAddChannel(t, 1, common.RoleRootUser, body)
		require.Contains(t, response.Body.String(), `"success":true`)
	}
	var created []model.Channel
	require.NoError(t, db.Where("name = ?", "new").Find(&created).Error)
	require.Len(t, created, 2)
	for _, ch := range created {
		require.Equal(t, "default", ch.Group)
	}
	clone := model.Channel{Name: "clone", Type: 1, Key: "test-key", Models: "gpt", Group: "noahgrape-azure", DispatchInfo: legacy.DispatchInfo}
	require.NoError(t, clone.Insert())
	require.Equal(t, "default", clone.Group)
	require.Empty(t, clone.DispatchInfo)
	edited := putUpdateChannel(t, 1, common.RoleRootUser, fmt.Sprintf(`{"id":%d,"group":"noahgrape-azure"}`, clone.Id))
	require.Contains(t, edited.Body.String(), `"success":true`)
	var updated model.Channel
	require.NoError(t, db.First(&updated, clone.Id).Error)
	require.Equal(t, "noahgrape-azure", updated.Group)
	request, err := model.RequestDispatchBinding(clone.Id, "noahgrape-azure", 99, 1)
	require.NoError(t, err)
	again, err := model.RequestDispatchBinding(clone.Id, "noahgrape-azure", 99, 1)
	require.NoError(t, err)
	require.Equal(t, request.Id, again.Id)
	_, err = model.RequestDispatchBinding(clone.Id, "noahgrape-azure", 100, 1)
	require.ErrorContains(t, err, "pending")
	require.NoError(t, db.Model(request).Update("status", "failed").Error)
	retried, err := model.RequestDispatchBinding(clone.Id, "noahgrape-azure", 100, 1)
	require.NoError(t, err)
	require.Equal(t, request.Id, retried.Id)
	require.NoError(t, db.AutoMigrate(&model.DispatchBindingRequest{}))
	var count int64
	require.NoError(t, db.Model(&model.DispatchBindingRequest{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	for _, body := range []string{
		`{"master_group":"noahgrape-azure","remote_channel_id":0}`,
		`{"master_group":"deleted-group","remote_channel_id":99}`,
		`{"master_group":"default","remote_channel_id":99}`,
		`{"master_group":"noahgrape-azure","remote_channel_id":1.5}`,
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("role", common.RoleRootUser)
		ctx.Set("id", 1)
		ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(clone.Id)}}
		ctx.Request = httptest.NewRequest("POST", "/api/channel/1/dispatch-bindings", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		BindChannelDispatch(ctx)
		require.Contains(t, recorder.Body.String(), `"success":false`)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(clone.Id)}}
	ctx.Request = httptest.NewRequest("GET", "/api/channel/1/dispatch-bindings", nil)
	GetChannelDispatchBindings(ctx)
	require.Contains(t, recorder.Body.String(), `"remote_channel_id":100`)
	require.NotContains(t, recorder.Body.String(), "private-key")
	// A fresh binding table also migrates twice and enforces target uniqueness.
	require.NoError(t, db.Migrator().DropTable(&model.DispatchBindingRequest{}))
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.DispatchBindingRequest{}))
	}
	_, err = model.RequestDispatchBinding(clone.Id, "noahgrape-azure", 99, 1)
	require.NoError(t, err)
	require.Error(t, db.Create(&model.DispatchBindingRequest{ChannelId: clone.Id, MasterGroup: "noahgrape-azure", RemoteChannelId: 100}).Error)
}
