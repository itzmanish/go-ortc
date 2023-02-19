package goortc

import (
	"strconv"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type Consumer struct {
	Id uint

	paused      atomicBool
	priority    uint
	kind        webrtc.RTPCodecType
	isSimulcast bool

	parameters RTPParameters
	track      *DownTrack
	producer   *Producer
	sender     *webrtc.RTPSender
	transport  *WebRTCTransport

	onRTPPacket func(id uint, packet *buffer.ExtPacket)
}

func newConsumer(id uint, producer *Producer, track *DownTrack, transport *WebRTCTransport, paused bool) (*Consumer, error) {
	sender, err := transport.router.api.NewRTPSender(track, transport.dtlsConn)
	if err != nil {
		return nil, errFailedToCreateConsumer(err)
	}

	parameters := sender.GetParameters()
	err = sender.Send(parameters)
	if err != nil {
		return nil, err
	}

	consumer := &Consumer{
		Id:        id,
		producer:  producer,
		sender:    sender,
		paused:    0,
		transport: transport,
		track:     track,
		parameters: RTPParameters{
			Codecs:           parameters.Codecs,
			HeaderExtensions: parameters.HeaderExtensions,
			Encodings:        parameters.Encodings,
			Mid:              strconv.Itoa(int(transport.getMid())),
		},
		onRTPPacket: nil,
	}
	if paused {
		consumer.paused.set(true)
	}
	return consumer, nil
}

func (c *Consumer) GetParameters() RTPParameters {
	return c.parameters
}

func (c *Consumer) Write(packet *buffer.ExtPacket) error {
	if c.paused.get() {
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
