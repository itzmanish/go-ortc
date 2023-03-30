package rtc

import (
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/livekit/mediatransportutil/pkg/bucket"
	"github.com/livekit/mediatransportutil/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"go.uber.org/atomic"
)

type Producer struct {
	logger.Logger
	Id uint

	closeOnce sync.Once
	closeCh   chan struct{}
	closed    atomic.Bool
	paused    atomic.Bool

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

func newProducer(id uint, receiver *webrtc.RTPReceiver, transport *WebRTCTransport, params RTPReceiveParameters) *Producer {
	track := receiver.Track()
	producer := &Producer{
		Id:         id,
		receiver:   receiver,
		transport:  transport,
		track:      track,
		kind:       MediaKind(track.Kind()),
		parameters: params,
		trackID:    uuid.NewString(),
		closeCh:    make(chan struct{}),
		closed:     *atomic.NewBool(false),
		paused:     *atomic.NewBool(false),
		// stream id needs to be generated as unique
		streamID: params.Rtcp.Cname,
		twcc:     transport.twcc,
		rtcpChan: make(chan []rtcp.Packet),
		Logger:   logger.NewLogger(fmt.Sprintf("Producer [id: %v]", id)).WithField("Kind", track.Kind().String()),
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
	if p.closed.Swap(true) {
		return
	}
	close(p.closeCh)
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.receiver.Stop()
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

func (p *Producer) Paused() bool {
	return p.paused.Load()
}

func (p *Producer) SendRTCP(packets []rtcp.Packet) {
	if packets == nil || p.closed.Load() {
		return
	}
	p.Logger.Debugf("PRODUCER::SendRTCP sending rtcp fb: %+v", packets)
	_, err := p.WriteRTCP(packets)
	if err != nil {
		p.Logger.Error("Producer::SendRTCP Error:", err)
	}
}

func (p *Producer) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	return p.transport.WriteRTCP(pkts)
}

func (p *Producer) GetRTCPSenderReportDataExt() *buffer.RTCPSenderReportDataExt {
	return p.buffer.GetSenderReportDataExt()
}

func (p *Producer) ReadRTP(dst []byte, sn uint16) (int, error) {
	return p.buffer.GetPacket(dst, sn)
}

func (p *Producer) readRTPWorker() {
	defer func() {
		p.Close()
	}()
	p.Logger.Infof("Producer %v: reading RTP packets", p.Id)
	for {
		select {
		case <-p.closeCh:
			p.Logger.Warn("producer close, stopping readRTPWorker")
			return
		default:
			pktBuf := make([]byte, bucket.MaxPktSize)
			pkt, err := p.buffer.ReadExtended(pktBuf)
			if err == io.EOF {
				p.Logger.Warn("Error reading from buffer, exiting..", err)
				return
			} else if err != nil {
				logger.Errorf("Producer %s: error reading RTP packets: %s", p.Id, err)
				return
			}
			if p.onRTPPacket != nil && !p.Paused() {
				p.onRTPPacket(p.Id, pkt)
			}
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
