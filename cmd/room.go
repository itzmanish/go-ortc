package main

import (
	"encoding/json"
	"fmt"

	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/itzmanish/go-ortc/pkg/rtc"
	"github.com/pion/webrtc/v3"
)

type Room struct {
	id     uint
	peers  map[uint]*Peer
	logger logger.Logger
	router *rtc.Router
}

func NewRoom(id uint, router *rtc.Router) *Room {
	return &Room{
		id:     id,
		peers:  map[uint]*Peer{},
		logger: logger.NewLogger(fmt.Sprintf("[Room: %v]", id)),
		router: router,
	}
}

func (r *Room) GetPeer(id uint) *Peer {
	return r.peers[id]
}

func (r *Room) GetPeerIds() []uint {
	ids := []uint{}
	for k := range r.peers {
		ids = append(ids, k)
	}
	return ids
}

func (r *Room) AddPeer(pId uint) *Peer {
	peer := r.GetPeer(pId)
	if peer != nil {
		peer.Close()
	}
	peer = NewPeer(pId, r.router)
	r.SetPeer(peer.Id, peer)
	return peer
}

func (r *Room) SetPeer(id uint, peer *Peer) {
	r.peers[id] = peer
}

func (r *Room) RemovePeer(id uint) {
	delete(r.peers, id)
}

func (r *Room) HandleIncomingMessage(peer *Peer, msg *WebSocketMessage, cb func([]byte, error)) {
	r.logger.Infof("Got incoming message %+v", msg)
	switch msg.MessageType {
	case JoinRoomMessage:
		{
			var payload struct {
				PeerID uint `json:"peerId"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			if peer := r.GetPeer(payload.PeerID); peer != nil {
				peer.Close()
			}
			peer := NewPeer(payload.PeerID, r.router)
			r.SetPeer(payload.PeerID, peer)
			res := struct {
				Peers []uint `json:"peers"`
			}{
				Peers: r.GetPeerIds(),
			}
			cb(BuildMessage(res))
			break
		}
	case LeaveRoomMessage:
		{

		}
	case GetRouterCapability:
		{
			caps := r.router.GetRouterCapabilities()
			cb(BuildMessage(caps))
			break
		}
	case CreateTransport:
		{
			var payload struct {
				Consuming bool `json:"consuming"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			transport, err := peer.CreateTransport(payload.Consuming)
			if err != nil {
				cb(nil, err)
				return
			}
			caps := transport.GetCapabilities()
			cb(BuildMessage(struct {
				TransportId string `json:"id"`
				rtc.TransportCapabilities
			}{
				TransportId:           fmt.Sprintf("%d", transport.Id),
				TransportCapabilities: caps,
			}))
			break
		}
	case ConnectTransport:
		{
			var payload struct {
				TransportId    uint                  `json:"transportId"`
				DtlsParameters webrtc.DTLSParameters `json:"dtlsParameters"`
				IceParameters  webrtc.ICEParameters  `json:"iceParameters"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			r.logger.Info("payload", payload)
			err = peer.ConnectTransport(payload.TransportId, payload.DtlsParameters, payload.IceParameters)
			if err != nil {
				cb(nil, err)
			}
			cb(nil, nil)
		}
	case CloseTransport:
		{
			var payload struct {
				TransportId uint `json:"transportId"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			err = peer.CloseTransport(payload.TransportId)
			cb(nil, err)
			break
		}
	case Produce:
		{
			var payload struct {
				Kind       string            `json:"kind"`
				Parameters rtc.RTPParameters `json:"rtpParameters"`
				Simulcast  bool              `json:"simulcast"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			producer, err := peer.Produce(payload.Kind, payload.Parameters, payload.Simulcast)
			if err != nil {
				cb(nil, err)
				return
			}
			cb(BuildMessage(struct {
				ProducerID uint `json:"producerId"`
			}{
				ProducerID: producer.Id,
			}))
			break
		}
	case CloseProducer:
		{

		}
	case ProducerToggle:
		{

		}
	default:
	}
}
