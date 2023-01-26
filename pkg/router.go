package goortc

import (
	"math"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/webrtc/v3"
)

// A router object is equivalent to a room
// all the data for same room can be forwarded to associated
// to and from consumer and producer respectively.
type Router struct {
	Id uint

	transports []*WebRTCTransport
	producers  []*Producer
	consumers  []*Consumer
	me         *webrtc.MediaEngine
	api        *webrtc.API

	currentTransportId uint
}

func NewRouter(id uint, me *webrtc.MediaEngine, api *webrtc.API) *Router {
	return &Router{
		Id:                 id,
		me:                 me,
		api:                api,
		transports:         []*WebRTCTransport{},
		producers:          []*Producer{},
		consumers:          []*Consumer{},
		currentTransportId: 0,
	}
}

func (router *Router) NewWebRTCTransport() (*WebRTCTransport, error) {
	transport, err := newWebRTCTransport(router.generateNewWebrtcTransportID(), router)
	if err != nil {
		return nil, err
	}
	router.transports = append(router.transports, transport)
	return transport, nil
}

func (router *Router) AddProducer(producer *Producer) error {
	producer.OnRTP(router.OnRTPPacket())
	router.producers = append(router.producers, producer)
	return nil
}

func (router *Router) OnRTPPacket() OnRTPPacketHandlerFunc {
	return func(rtp *buffer.ExtPacket) {

	}
}

func (router *Router) generateNewWebrtcTransportID() uint {
	if router.currentTransportId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	router.currentTransportId += 1
	return router.currentTransportId
}
