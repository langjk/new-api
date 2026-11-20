package controller

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAzureQuotaMatching(t *testing.T) {
	for _, tc := range []struct {
		name, meta, region, tier string
		want                     bool
	}{
		{"lowest tier", `[{"tier":5},{"tier":2}]`, "", "2", true},
		{"higher tier excluded", `[{"tier":5},{"tier":2}]`, "", "5", false},
		{"PTU excluded from minimum", `[{"tier":1,"offer":" PTUM "},{"tier":4}]`, "", "4", true},
		{"PTU separately selectable", `[{"tier":1,"offer":"ptum"},{"tier":4}]`, "", "ptu", true},
		{"legacy object", `{"tier":3,"region":" EastUS "}`, "eastus", "3", true},
		{"any deployment region", `[{"tier":2,"region":"eastus"},{"tier":4,"region":"westus"}]`, "westus", "2", true},
		{"region exact match", `{"tier":2,"region":"eastus2"}`, "eastus", "2", false},
		{"missing metadata", ``, "", "1", false},
		{"malformed metadata", `[broken`, "", "1", false},
		{"invalid entry skipped", `[null,1,{"tier":3}]`, "", "3", true},
		{"non-integer ignored", `[{"tier":2.5}]`, "", "2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, channelMatchesAzureQuota(&model.Channel{AzureMeta: &tc.meta}, tc.region, tc.tier))
		})
	}
}

// Run against SQLite by default, plus isolated real engines via TEST_CHANNEL_SQL_DSN.
func TestChannelAzureFiltersDatabase(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldMaster, oldPath := common.IsMasterNode, common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.IsMasterNode, common.SQLitePath = oldMaster, oldPath
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false
	common.SQLitePath = filepath.Join(t.TempDir(), "azure-filters.db")
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	versionQuery := "SELECT VERSION()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database=%s version=%s", common.MainDatabaseType(), version)
	metas := []string{
		`[{"tier":5,"region":"EastUS"},{"tier":2,"region":"EastUS"}]`,
		`{"tier":2,"region":"eastus"}`,
		`{"tier":4,"region":"westus"}`,
		`{"tier":2,"region":"private-region"}`,
		`{"tier":2,"region":"eastus"}`,
		`{"offer":"ptum","region":"eastus","tier":1}`,
	}
	channels := make([]model.Channel, len(metas))
	tag := "azure-filter-test"
	for i := range channels {
		channels[i] = model.Channel{Name: "azure-filter-test", Type: 3, CreatorId: 2, Status: 1, Key: "must-not-leak", Group: "vip", Models: "gpt-test", AzureMeta: &metas[i], Tag: &tag}
	}
	channels[1].Type = 14 // Azure-hosted providers retain their protocol type.
	channels[3].CreatorId = 3
	channels[4].Status = 2
	require.NoError(t, db.Create(&channels).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&channels).Error) })
	for _, tc := range []struct {
		name, query string
		list        bool
		role        int
		total       int
		ids         []int
	}{
		{"list regions cover other pages and exclude other owners", "?p=1&page_size=1&id_sort=true", true, common.RoleAdminUser, 5, []int{channels[5].Id}},
		{"combined filters page one", "?azure_region=EASTUS&azure_tier=2&status=enabled&group=vip&model=gpt-test&id_sort=true&page_size=1", false, common.RoleAdminUser, 2, []int{channels[1].Id}},
		{"combined filters page two", "?azure_region=eastus&azure_tier=2&status=enabled&group=vip&id_sort=true&page_size=1&p=2", false, common.RoleAdminUser, 2, []int{channels[0].Id}},
		{"list endpoint honors filters", "?azure_region=eastus&azure_tier=2&status=enabled&type=14", true, common.RoleAdminUser, 1, []int{channels[1].Id}},
		{"tag search honors filters", "?azure_region=eastus&azure_tier=2&status=enabled&type=3&tag_mode=true", false, common.RoleAdminUser, 1, []int{channels[0].Id}},
		{"PTU selection", "?azure_tier=ptu", false, common.RoleAdminUser, 1, []int{channels[5].Id}},
		{"higher deployment tier does not override displayed minimum", "?azure_tier=5", false, common.RoleAdminUser, 0, []int{}},
		{"foreign region excluded for admin", "?azure_region=private-region", false, common.RoleAdminUser, 0, []int{}},
		{"foreign region excluded for ordinary user", "?azure_region=private-region", false, common.RoleCommonUser, 0, []int{}},
		{"root can filter all owners", "?azure_region=private-region", false, common.RoleRootUser, 1, []int{channels[3].Id}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("GET", "/api/channel/"+tc.query, nil)
			ctx.Set("id", 2)
			ctx.Set("role", tc.role)
			if tc.list {
				GetAllChannels(ctx)
			} else {
				SearchChannels(ctx)
			}
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Items      []*model.Channel `json:"items"`
					Total      int              `json:"total"`
					Regions    []string         `json:"azure_regions"`
					TypeCounts map[int]int      `json:"type_counts"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			assert.Equal(t, tc.total, response.Data.Total)
			ids := make([]int, 0, len(response.Data.Items))
			for _, channel := range response.Data.Items {
				ids = append(ids, channel.Id)
				assert.Empty(t, channel.Key)
			}
			assert.Equal(t, tc.ids, ids)
			assert.Contains(t, response.Data.Regions, "westus")
			if tc.role < common.RoleRootUser {
				assert.NotContains(t, response.Data.Regions, "private-region")
			}
			if tc.name == "combined filters page one" {
				assert.Equal(t, map[int]int{3: 1, 14: 1}, response.Data.TypeCounts)
			}
		})
	}
	for _, tier := range []string{"0", "7", "2.5", "oops"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("GET", "/api/channel/search?azure_tier="+tier, nil)
		SearchChannels(ctx)
		assert.Contains(t, recorder.Body.String(), `"success":false`)
	}
}
