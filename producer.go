package goortc

import (
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type Producer struct {
	Id uint

	closeOnce sync.Once

	parameters RTPParameters
	receiver   *webrtc.RTPReceiver
	transport  *WebRTCTransport

	track        *webrtc.TrackRemote
	trackID      string
	streamID     string
	onRTPHandler OnRTPHandlerFunc
}

type OnRTPHandlerFunc func(rtp *rtp.Packet)

func newProducer(id uint, receiver *webrtc.RTPReceiver, transport *WebRTCTransport, params RTPParameters) *Producer {
	track := receiver.Track()
	return &Producer{
		Id:         id,
		receiver:   receiver,
		transport:  transport,
		track:      track,
		parameters: params,
		trackID:    track.ID(),
		streamID:   track.StreamID(),
	}
}

func (p *Producer) SetTrackMeta(trackId, streamId string) {
	p.trackID = trackId
	p.streamID = streamId
}

func (p *Producer) SSRC() webrtc.SSRC {
	return p.track.SSRC()
}

func (p *Producer) OnRTP(h OnRTPHandlerFunc) {
	p.onRTPHandler = h
}
