package goortc

import (
	"io"
	"sync"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type Producer struct {
	Id uint

	closed      atomicBool
	closeOnce   sync.Once
	paused      atomicBool
	isSimulcast bool

	parameters RTPParameters
	receiver   *webrtc.RTPReceiver
	transport  *WebRTCTransport
	buffers    *buffer.Buffer
	rtcpReader *buffer.RTCPReader

	track          *webrtc.TrackRemote
	trackID        string
	streamID       string
	onRTPPacket    OnRTPPacketHandlerFunc
	onRTCPPacket   OnRTCPPacketHandlerFunc
	onCloseHandler OnProducerCloseHandlerFunc
}

type OnRTPPacketHandlerFunc func(producerId uint, rtp *buffer.ExtPacket)
type OnRTCPPacketHandlerFunc func(producerId uint, rtcp *rtcp.Packet)
type OnProducerCloseHandlerFunc func()

func newProducer(id uint, receiver *webrtc.RTPReceiver, transport *WebRTCTransport, params RTPParameters, simulcast bool) *Producer {
	track := receiver.Track()
	producer := &Producer{
		Id:          id,
		receiver:    receiver,
		transport:   transport,
		track:       track,
		parameters:  params,
		trackID:     track.ID(),
		streamID:    track.StreamID(),
		isSimulcast: simulcast,
	}
	return producer
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
		logger.Infof("Producer %v: reading RTP packets", p.Id)
		pkt, err := p.buffers.ReadExtended()
		if err == io.EOF {
			return
		} else if err != nil {
			logger.Errorf("Producer %s: error reading RTP packets: %s", p.Id, err)
			return
		}
		logger.Info("readRTP", "pkt", pkt)
		if p.onRTPPacket != nil {
			p.onRTPPacket(p.Id, pkt)
		}
	}
}

func (p *Producer) SendRTCP(pkts []rtcp.Packet) {
	p.transport.WriteRTCP(pkts)
}

func (p *Producer) closeTrack() {

}
