package model

import (
	_ "embed"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const DispatchAzureTiersOption = "DispatchAzureTierTable"

// GlobalStandard TPM thresholds from Microsoft, verified 2026-11-19.
//
//go:embed azure_tiers_defaults.json
var defaultAzureTiers []byte

var ErrAzureTiersInvalid = errors.New("Invalid Azure Tier table: use unique model names and nondecreasing positive TPM thresholds")
var ErrAzureTiersConflict = errors.New("Azure Tier table changed. Reload before saving")

type AzureTierRow struct {
	Model string  `json:"model"`
	TPM   []int64 `json:"tpm"` // Tier 1–6; zero means no published threshold.
}

type AzureTierTable struct {
	Revision string         `json:"revision"`
	Rows     []AzureTierRow `json:"rows"`
}

func GetAzureTierTable() (AzureTierTable, error) {
	var option Option
	err := DB.Where(&Option{Key: DispatchAzureTiersOption}).First(&option).Error
	var table AzureTierTable
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && option.Value == "") {
		err = common.Unmarshal(defaultAzureTiers, &table.Rows)
		return table, err
	}
	if err != nil {
		return table, err
	}
	err = common.UnmarshalJsonStr(option.Value, &table)
	return table, err
}

func SaveAzureTierTable(table AzureTierTable) (AzureTierTable, error) {
	if table.Rows == nil || len(table.Rows) > 500 || len(table.Revision) > 36 {
		return table, ErrAzureTiersInvalid
	}
	seen := map[string]bool{}
	for i := range table.Rows {
		row := &table.Rows[i]
		row.Model = strings.ToLower(strings.TrimSpace(row.Model))
		if row.Model == "" || len(row.Model) > 200 || strings.ContainsAny(row.Model, "\t\r\n ,") || seen[row.Model] || len(row.TPM) != 6 {
			return table, ErrAzureTiersInvalid
		}
		seen[row.Model] = true
		var previous int64
		for _, limit := range row.TPM {
			if limit < 0 || limit > 1000000000000 || (limit > 0 && limit < previous) {
				return table, ErrAzureTiersInvalid
			}
			if limit > 0 {
				previous = limit
			}
		}
		if previous == 0 {
			return table, ErrAzureTiersInvalid
		}
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var option Option
		if err := tx.Where(&Option{Key: DispatchAzureTiersOption}).FirstOrCreate(&option, Option{Key: DispatchAzureTiersOption}).Error; err != nil {
			return err
		}
		var current AzureTierTable
		if option.Value != "" {
			if err := common.UnmarshalJsonStr(option.Value, &current); err != nil {
				return err
			}
		}
		if current.Revision != table.Revision {
			return ErrAzureTiersConflict
		}
		table.Revision = uuid.NewString()
		encoded, err := common.Marshal(table)
		if err != nil {
			return err
		}
		result := tx.Model(&Option{}).Where(&Option{Key: DispatchAzureTiersOption}).Where("value = ?", option.Value).Update("value", string(encoded))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAzureTiersConflict
		}
		return nil
	})
	return table, err
}
