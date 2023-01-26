package goortc

import (
	"io"
	"sync"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/webrtc/v3"
)

type Producer struct {
	Id uint

	closed    atomicBool
	closeOnce sync.Once
	paused    atomicBool

	parameters RTPParameters
	receiver   *webrtc.RTPReceiver
	transport  *WebRTCTransport
	buffers    *buffer.Buffer

	track          *webrtc.TrackRemote
	trackID        string
	streamID       string
	onRTPPacket    OnRTPPacketHandlerFunc
	onCloseHandler OnProducerCloseHandlerFunc
}

type OnRTPPacketHandlerFunc func(rtp *buffer.ExtPacket)
type OnProducerCloseHandlerFunc func()

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

func (p *Producer) OnRTP(h OnRTPPacketHandlerFunc) {
	p.onRTPPacket = h
}

func (p *Producer) Close() {
	if p.closed.get() {
		return
	}

	p.closeOnce.Do(func() {
		p.closed.set(true)
		p.closeTrack()
	})
	if p.onCloseHandler != nil {
		p.onCloseHandler()
	}
}

func (p *Producer) Pause(value bool) {
	if p.closed.get() {
		return
	}

	p.paused.set(value)
}

func (p *Producer) readRTP() {
	// Read RTP packets from the receiver's track and send them to the router
	// FIXME: should need to implement a way to stop the goroutine
	defer func() {
		p.closeOnce.Do(func() {
			p.closed.set(true)
			p.closeTrack()
		})
	}()
	for {
		logger.Infof("Producer %s: reading RTP packets", p.Id)
		pkt, err := p.buffers.ReadExtended()
		if err == io.EOF {
			return
		} else if err != nil {
			logger.Errorf("Producer %s: error reading RTP packets: %s", p.Id, err)
			return
		}
		logger.Info("readRTP", "pkt", pkt)
		if p.onRTPPacket != nil {
			p.onRTPPacket(pkt)
		}
	}
}

func (p *Producer) closeTrack() {

}
