package rtc

import (
	"strconv"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type Consumer struct {
	Id uint

	paused      bool
	priority    uint
	kind        MediaKind
	isSimulcast bool

	parameters RTPSendParameters
	track      *DownTrack
	producer   *Producer
	sender     *webrtc.RTPSender
	transport  *WebRTCTransport

	onRTPPacket func(id uint, packet *buffer.ExtPacket)
}

func newConsumer(id uint, producer *Producer, track *DownTrack, transport *WebRTCTransport, paused bool) (*Consumer, error) {
	sender, err := transport.api.NewRTPSender(track, transport.dtlsConn)
	if err != nil {
		return nil, errFailedToCreateConsumer(err)
	}

	parameters := sender.GetParameters()
	err = sender.Send(parameters)
	if err != nil {
		return nil, err
	}
	ortcParams := ParseRTPSendParametersToORTC(parameters)
	ortcParams.Rtcp.Cname = track.streamID
	ortcParams.Mid = strconv.Itoa(int(transport.getMid()))
	consumer := &Consumer{
		Id:          id,
		producer:    producer,
		sender:      sender,
		paused:      paused,
		transport:   transport,
		track:       track,
		kind:        producer.kind,
		parameters:  ortcParams,
		onRTPPacket: nil,
	}
	return consumer, nil
}

func (c *Consumer) Kind() MediaKind {
	return c.kind
}

func (c *Consumer) GetParameters() RTPSendParameters {
	return c.parameters
}

func (c *Consumer) Resume() {
	c.paused = false
}

func (c *Consumer) Pause() {
	c.paused = true
}

func (c *Consumer) Write(packet *buffer.ExtPacket) error {
	if c.paused {
		return nil
	}
	err := c.track.writeSimpleRTP(packet)
	if err != nil {
		return err
	}
	if c.onRTPPacket != nil {
		c.onRTPPacket(c.Id, packet)
	}
	return nil
}

func (c *Consumer) SendRTCP(packets []rtcp.Packet) (int, error) {
	return c.transport.WriteRTCP(packets)
}
