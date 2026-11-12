package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestChannelOwnershipDatabaseMatrix(t *testing.T) {
	drivers := map[string]gorm.Dialector{"sqlite": sqlite.Open(filepath.Join(t.TempDir(), "test.db"))}
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		drivers["mysql"] = mysql.Open(dsn)
	}
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		drivers["postgres"] = postgres.Open(dsn)
	}
	oldDB := model.DB
	defer func() { model.DB = oldDB }()
	for name, driver := range drivers {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			defer sqlDB.Close()
			require.NoError(t, db.AutoMigrate(&model.Channel{}))
			model.DB = db
			own := model.Channel{Name: "matrix-own", CreatorId: 2, Key: "hidden"}
			foreign := model.Channel{Name: "matrix-foreign", CreatorId: 3, Key: "hidden"}
			require.NoError(t, db.Create(&own).Error)
			require.NoError(t, db.Create(&foreign).Error)
			defer db.Delete(&own)
			defer db.Delete(&foreign)
			var found []model.Channel
			require.NoError(t, applyChannelOwnerFilter(db.Model(&model.Channel{}), common.RoleAdminUser, 2).Find(&found).Error)
			require.Len(t, found, 1)
			assert.Equal(t, own.Id, found[0].Id)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("id", 2)
			ctx.Set("role", common.RoleAdminUser)
			assert.True(t, callerOwnsChannels(ctx, own.Id))
			assert.False(t, callerOwnsChannels(ctx, foreign.Id))
		})
	}
}
