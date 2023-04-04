package rtc

import (
	"fmt"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/pion/webrtc/v3"
)

type DataProducer struct {
	Id uint
	logger.Logger

	channel          *webrtc.DataChannel
	onMessageHandler func(msg webrtc.DataChannelMessage)
	params           SCTPStreamParameters
	label            string
	messageRecieved  uint
	bytesRecieved    uint
}

func NewDataProducer(id uint, label string, params SCTPStreamParameters) *DataProducer {
	return &DataProducer{
		Id:              id,
		params:          params,
		label:           label,
		messageRecieved: 0,
		bytesRecieved:   0,
		Logger:          logger.NewLogger(fmt.Sprintf("DataProducer [id: %s]", id)).WithField("streamId", params.StreamID),
	}
}

func (dp *DataProducer) SetDataChannel(ch *webrtc.DataChannel) {
	dp.channel = ch
	ch.OnMessage(dp.handleMessage)
}

func (dp *DataProducer) OnMessage(fn func(webrtc.DataChannelMessage)) {
	dp.onMessageHandler = fn
}

func (dp *DataProducer) handleMessage(msg webrtc.DataChannelMessage) {
	dp.Info("got message on data producer", msg)
	if dp.onMessageHandler != nil {
		dp.onMessageHandler(msg)
	}
}
