package model

import (
	_ "embed"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

// RecordAzureTestMetadata uses the successful manual test's response, without
// probing again. Only the tested model is replaced; other models keep their data.
func RecordAzureTestMetadata(channel *Channel, testedModel string, headers map[string]string, testedAt int64) error {
	if channel == nil || channel.Type != constant.ChannelTypeAzure || testedModel == "" {
		return nil
	}
	tpm, err := strconv.ParseInt(strings.TrimSpace(headers["x-ratelimit-limit-tokens"]), 10, 64)
	if err != nil || tpm <= 0 {
		return nil // Missing/invalid quota is not evidence that the previous tier changed.
	}
	table, err := GetAzureTierTable()
	if err != nil {
		return err
	}
	entry := map[string]any{"model": testedModel, "tpm": tpm, "probed_at": testedAt, "status": 200}
	if rpm, err := strconv.ParseInt(strings.TrimSpace(headers["x-ratelimit-limit-requests"]), 10, 64); err == nil && rpm > 0 {
		entry["rpm"] = rpm
	}
	if region := strings.ToLower(strings.Join(strings.Fields(headers["x-ms-region"]), "")); region != "" {
		entry["region"] = region
	}
	offer := strings.ToLower(strings.TrimSpace(headers["azureai-fe-offer-type"]))
	if offer != "" {
		entry["offer"] = offer
	}
	if offer != "ptum" {
		quotaModel := testedModel
		if channel.ModelMapping != nil {
			var mapping map[string]string
			if common.UnmarshalJsonStr(*channel.ModelMapping, &mapping) == nil && mapping[testedModel] != "" {
				quotaModel = mapping[testedModel]
			}
		}
		for _, row := range table.Rows {
			if !strings.EqualFold(row.Model, strings.TrimSpace(quotaModel)) {
				continue
			}
			// Repeated thresholds use the lowest matching tier, as in the
			// previous dispatcher classifier. Sub-Tier-1 positive TPM is Tier 1.
			tier, reached := 1, int64(0)
			for i, limit := range row.TPM {
				if limit > reached && limit <= tpm {
					tier, reached = i+1, limit
				}
			}
			entry["tier"] = tier
			break
		}
	}
	for range 5 {
		var current Channel
		if err := DB.Select("id", "type", "azure_meta").First(&current, channel.Id).Error; err != nil {
			return err
		}
		if current.Type != constant.ChannelTypeAzure {
			return nil
		}
		var entries []map[string]any
		if current.AzureMeta != nil && strings.TrimSpace(*current.AzureMeta) != "" {
			if err := common.UnmarshalJsonStr(*current.AzureMeta, &entries); err != nil {
				var legacy map[string]any
				if err := common.UnmarshalJsonStr(*current.AzureMeta, &legacy); err != nil {
					return err
				}
				entries = []map[string]any{legacy}
			}
		}
		merged := make([]map[string]any, 0, len(entries)+1)
		for _, previous := range entries {
			if name, _ := previous["model"].(string); strings.EqualFold(name, testedModel) {
				if timestamp, _ := previous["probed_at"].(float64); timestamp > float64(testedAt) {
					return nil // A later manual test has already finished.
				}
				continue
			}
			merged = append(merged, previous)
		}
		merged = append(merged, entry)
		encoded, err := common.Marshal(merged)
		if err != nil {
			return err
		}
		if current.AzureMeta != nil && *current.AzureMeta == string(encoded) {
			return nil
		}
		query := DB.Model(&Channel{}).Where("id = ? AND type = ?", channel.Id, constant.ChannelTypeAzure)
		if current.AzureMeta == nil {
			query = query.Where("azure_meta IS NULL")
		} else {
			query = query.Where("azure_meta = ?", *current.AzureMeta)
		}
		result := query.Update("azure_meta", string(encoded))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			return nil
		}
	}
	return errors.New("Azure metadata changed concurrently; retry the manual test")
}

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
