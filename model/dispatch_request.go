package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// DispatchRequest 前端「重推」请求：把渠道的某个下发落点（下游 + 下游分组）重新推送一次。
// new-api 只负责写入；外部 dispatcher 旁车轮询消费后删除。
type DispatchRequest struct {
	Id              int    `json:"id"`
	ChannelId       int    `json:"channel_id" gorm:"index"`
	Downstream      string `json:"downstream" gorm:"type:varchar(64)"`
	DownstreamGroup string `json:"downstream_group" gorm:"type:varchar(255)"`
	UserId          int    `json:"user_id"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint"`
}

// CreateDispatchRequest 写入一条重推请求；同一落点已有未消费的请求时不重复写。
func CreateDispatchRequest(channelId int, downstream string, downstreamGroup string, userId int) error {
	var count int64
	err := DB.Model(&DispatchRequest{}).
		Where("channel_id = ? AND downstream = ? AND downstream_group = ?", channelId, downstream, downstreamGroup).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return DB.Create(&DispatchRequest{
		ChannelId:       channelId,
		Downstream:      downstream,
		DownstreamGroup: downstreamGroup,
		UserId:          userId,
		CreatedTime:     common.GetTimestamp(),
	}).Error
}

// DispatchBindingRequest asks the dispatcher to verify and adopt an existing ID.
// It never creates or edits a channel at the downstream service.
type DispatchBindingRequest struct {
	Id              int    `json:"id"`
	ChannelId       int    `json:"channel_id" gorm:"uniqueIndex:idx_dispatch_binding_target"`
	MasterGroup     string `json:"master_group" gorm:"type:varchar(255);uniqueIndex:idx_dispatch_binding_target"`
	RemoteChannelId int    `json:"remote_channel_id"`
	Status          string `json:"status" gorm:"type:varchar(16);index"`
	Message         string `json:"message" gorm:"type:text"`
	UserId          int    `json:"-"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint"`
}

func RequestDispatchBinding(channelID int, masterGroup string, remoteID int, userID int) (*DispatchBindingRequest, error) {
	var request DispatchBindingRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id").First(&channel, channelID).Error; err != nil {
			return err
		}
		err := tx.Where("channel_id = ? AND master_group = ?", channelID, masterGroup).First(&request).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if request.Id != 0 {
			if request.RemoteChannelId == remoteID && request.Status != "failed" {
				return nil
			}
			if request.Status == "pending" {
				return common.NewMessage("A binding request is already pending")
			}
		}
		request.ChannelId, request.MasterGroup, request.RemoteChannelId = channelID, masterGroup, remoteID
		request.Status, request.Message = "pending", ""
		request.UserId, request.CreatedTime = userID, common.GetTimestamp()
		return tx.Save(&request).Error
	})
	return &request, err
}
