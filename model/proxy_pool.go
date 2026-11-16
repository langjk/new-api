package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const DispatchProxyPoolOption = "DispatchProxyPool"

var ErrProxyPoolConflict = errors.New("Proxy pool changed. Reload before saving")
var ErrProxyPoolInvalid = errors.New("Invalid proxy pool: use unique proxy URLs and names, up to 500 entries")

type DispatchProxy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Proxy   string `json:"proxy"`
	Enabled bool   `json:"enabled"`
}

type DispatchProxyPool struct {
	Revision string          `json:"revision"`
	Proxies  []DispatchProxy `json:"proxies"`
}

func GetDispatchProxyPool() (DispatchProxyPool, error) {
	pool := DispatchProxyPool{Proxies: []DispatchProxy{}}
	var option Option
	err := DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
		Where(&Option{Key: DispatchProxyPoolOption}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pool, nil
	}
	if err != nil {
		return pool, err
	}
	if option.Value != "" {
		err = common.UnmarshalJsonStr(option.Value, &pool)
	}
	return pool, err
}

// SaveDispatchProxyPool uses the existing options table and an optimistic update
// so a stale browser cannot overwrite changes made by another root session.
func SaveDispatchProxyPool(pool DispatchProxyPool) (DispatchProxyPool, error) {
	if len(pool.Proxies) > 500 || pool.Proxies == nil || len(pool.Revision) > 36 {
		return pool, ErrProxyPoolInvalid
	}
	urls, ids := map[string]bool{}, map[string]bool{}
	for i := range pool.Proxies {
		p := &pool.Proxies[i]
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" || len(p.Name) > 100 || len(p.Proxy) > 2048 {
			return pool, ErrProxyPoolInvalid
		}
		parsed, err := common.ParseProxyURLStrict(p.Proxy)
		if err != nil || parsed == nil {
			return pool, ErrProxyPoolInvalid
		}
		parsed.Host = strings.ToLower(parsed.Host)
		p.Proxy = parsed.String()
		if p.ID == "" {
			p.ID = uuid.NewString()
		}
		if _, err := uuid.Parse(p.ID); err != nil || ids[p.ID] || urls[p.Proxy] {
			return pool, ErrProxyPoolInvalid
		}
		ids[p.ID], urls[p.Proxy] = true, true
	}
	err := DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Transaction(func(tx *gorm.DB) error {
		var option Option
		if err := tx.Where(&Option{Key: DispatchProxyPoolOption}).FirstOrCreate(&option, Option{Key: DispatchProxyPoolOption}).Error; err != nil {
			return err
		}
		var current DispatchProxyPool
		if option.Value != "" {
			if err := common.UnmarshalJsonStr(option.Value, &current); err != nil {
				return err
			}
		}
		if pool.Revision != current.Revision {
			return ErrProxyPoolConflict
		}
		pool.Revision = uuid.NewString()
		encoded, err := common.Marshal(pool)
		if err != nil {
			return err
		}
		result := tx.Model(&Option{}).Where(&Option{Key: DispatchProxyPoolOption}).
			Where("value = ?", option.Value).Update("value", string(encoded))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrProxyPoolConflict
		}
		return nil
	})
	return pool, err
}
