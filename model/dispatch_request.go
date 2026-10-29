package model

import "github.com/QuantumNous/new-api/common"

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
