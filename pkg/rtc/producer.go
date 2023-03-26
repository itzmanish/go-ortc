package rtc

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/livekit/mediatransportutil/pkg/bucket"
	"github.com/livekit/mediatransportutil/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type Producer struct {
	logger.Logger
	Id uint

	closeOnce   sync.Once
	closed      atomic.Bool
	paused      atomic.Bool
	isSimulcast bool

	parameters RTPReceiveParameters
	kind       MediaKind
	receiver   *webrtc.RTPReceiver
	transport  *WebRTCTransport
	buffer     *buffer.Buffer
	rtcpChan   chan []rtcp.Packet
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

func newProducer(id uint, receiver *webrtc.RTPReceiver, transport *WebRTCTransport, params RTPReceiveParameters, simulcast bool) *Producer {
	track := receiver.Track()
	producer := &Producer{
		Id:         id,
		receiver:   receiver,
		transport:  transport,
		track:      track,
		kind:       MediaKind(track.Kind()),
		parameters: params,
		trackID:    uuid.NewString(),
		// stream id needs to be generated as unique
		streamID:    params.Rtcp.Cname,
		isSimulcast: simulcast,
		twcc:        transport.twcc,
		rtcpChan:    make(chan []rtcp.Packet),
		Logger:      logger.NewLogger(fmt.Sprintf("Producer [id: %v]", id)).WithField("Kind", track.Kind().String()),
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
	if p.closed.Load() {
		return
	}

	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.closeTrack()
	})
	if p.onCloseHandler != nil {
		p.onCloseHandler()
	}
}

func (p *Producer) Pause(value bool) {
	if p.closed.Load() {
		return
	}

	p.paused.Store(value)
}

func (p *Producer) SendRTCP(packets []rtcp.Packet) {
	if packets == nil || p.closed.Load() {
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

func (p *Producer) GetRTCPSenderReportDataExt() *buffer.RTCPSenderReportDataExt {
	return p.buffer.GetSenderReportDataExt()
}
func (p *Producer) closeTrack() {

}

func (p *Producer) readRTP() {
	defer func() {
		p.closeOnce.Do(func() {
			p.closed.Store(true)
			p.closeTrack()
		})
	}()
	logger.Infof("Producer %v: reading RTP packets", p.Id)
	for {
		pktBuf := make([]byte, bucket.MaxPktSize)
		pkt, err := p.buffer.ReadExtended(pktBuf)
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

func (p *Producer) handleRTCP(bytes []byte) {
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
		default:
			logger.Infof("PRODUCER::handleRTCP, got rtcp packet: %+v", pkt)
		}
	}

}
