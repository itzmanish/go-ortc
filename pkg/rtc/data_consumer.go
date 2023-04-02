package rtc

import (
	"fmt"

	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/webrtc/v3"
)

type DataConsumer struct {
	Id uint
	logger.Logger

	channel            *webrtc.DataChannel
	params             SCTPStreamParameters
	transportConnected bool
	sctpAssociated     bool
	label              string
	maxMessageSize     uint32
	messageSent        uint
	bytesSent          uint
}

func NewDataConsumer(id uint, maxMessageSize uint32, channel *webrtc.DataChannel) *DataConsumer {
	dc := &DataConsumer{
		Id:             id,
		channel:        channel,
		label:          channel.Label(),
		maxMessageSize: maxMessageSize,
		params: SCTPStreamParameters{
			StreamID:          uint(*channel.ID()),
			MaxPacketLifeTime: uint(*channel.MaxPacketLifeTime()),
			MaxRetransmits:    uint(*channel.MaxRetransmits()),
			Ordered:           channel.Ordered(),
		},
		Logger: logger.NewLogger(fmt.Sprintf("DataConsumer [id: %v]", id)),
	}
	channel.OnOpen(dc.handleOpen)
	return dc
}

func (dc *DataConsumer) SetTranportConnected(value bool) {
	dc.transportConnected = value
}

func (dc *DataConsumer) IsActive() bool {
	return dc.transportConnected && dc.sctpAssociated
}

func (dc *DataConsumer) Label() string {
	return dc.label
}

func (dc *DataConsumer) GetParameters() SCTPStreamParameters {
	return dc.params
}

func (dc *DataConsumer) Send(msg []byte) error {
	if !dc.IsActive() {
		return fmt.Errorf("data consumer is not active")
	}

	if len(msg) > int(dc.maxMessageSize) {
		return fmt.Errorf("message size: %v is bigger than configured maxMessageSize: %v", len(msg), dc.maxMessageSize)
	}
	dc.bytesSent += uint(len(msg))
	dc.messageSent += 1
	return dc.channel.Send(msg)
}

func (dc *DataConsumer) handleOpen() {
	dc.Logger.Info("dc opened")
	dc.sctpAssociated = true
}
