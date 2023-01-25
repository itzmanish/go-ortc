package goortc

import "github.com/pion/webrtc/v3"

type DataProducer struct {
	id uint

	rtpSender *webrtc.RTPSender
}
