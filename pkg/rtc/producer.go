package rtc

import (
	"io"
	"sync"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/livekit/mediatransportutil/pkg/twcc"
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
	buffer     *buffer.Buffer
	rtcpChan   chan []rtcp.Packet
	rtcpReader *buffer.RTCPReader
	twcc       *twcc.Responder

	track          *webrtc.TrackRemote
	trackID        string
	streamID       string
	onRTPPacket    OnRTPPacketHandlerFunc
	onRTCPPacket   OnRTCPPacketHandlerFunc
	onCloseHandler OnProducerCloseHandlerFunc
}

type OnRTPPacketHandlerFunc func(producerId uint, rtp *buffer.ExtPacket)
type OnRTCPPacketHandlerFunc func(producerId uint, rtcp []rtcp.Packet)
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
		twcc:        transport.twcc,
		rtcpChan:    make(chan []rtcp.Packet),
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

func (p *Producer) OnRTCP(h OnRTCPPacketHandlerFunc) {
	p.onRTCPPacket = h
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
	logger.Debugf("Producer %v: reading RTP packets", p.Id)
	for {
		pkt, err := p.buffer.ReadExtended()
		if err == io.EOF {
			return
		} else if err != nil {
			logger.Errorf("Producer %s: error reading RTP packets: %s", p.Id, err)
			return
		}
		if p.onRTPPacket != nil {
			p.onRTPPacket(p.Id, pkt)
		}
	}
}

func (p *Producer) handleRTCP() {
	p.rtcpReader.OnPacket(func(bytes []byte) {
		pkts, err := rtcp.Unmarshal(bytes)
		if err != nil {
			logger.Error("could not unmarshal RTCP", err)
			return
		}

		for _, pkt := range pkts {
			switch pkt := pkt.(type) {
			case *rtcp.SourceDescription:
			// do nothing for now
			case *rtcp.SenderReport:
				p.buffer.SetSenderReportData(pkt.RTPTime, pkt.NTPTime)
			case *rtcp.TransportLayerCC:
				logger.Infof("PRODUCER::handleRTCP, got rtcp packet: %+v", pkt)
			}
		}
	})
}

func (p *Producer) SendRTCP(packets []rtcp.Packet) {
	if packets == nil || p.closed.get() {
		return
	}
	logger.Debugf("PRODUCER::SendRTCP sending rtcp fb: %+v", packets)
	_, err := p.WriteRTCP(packets)
	if err != nil {
		logger.Error("Producer::SendRTCP Error:", err)
	}
}

func (p *Producer) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	return p.transport.WriteRTCP(pkts)
}

func (p *Producer) closeTrack() {

}
