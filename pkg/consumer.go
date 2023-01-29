package goortc

import (
	"strconv"

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
	}
	if paused {
		consumer.paused.set(true)
	}
	return consumer, nil
}
