package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/dtls"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/ice"
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
	r.logger.Debugf("Got incoming message %+v", msg)
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
				TransportId    uint                `json:"transportId"`
				DtlsParameters dtls.DTLSParameters `json:"dtlsParameters"`
				IceParameters  ice.ICEParameters   `json:"iceParameters"`
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
				Kind       string                   `json:"kind"`
				Parameters rtc.RTPReceiveParameters `json:"rtpParameters"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			producer, err := peer.Produce(payload.Kind, payload.Parameters)
			if err != nil {
				cb(nil, err)
				return
			}
			cb(BuildMessage(struct {
				ProducerID uint `json:"id"`
			}{
				ProducerID: producer.Id,
			}))
			break
		}
	case ProduceData:
		{
			var payload struct {
				Label  string                   `json:"label"`
				Params rtc.SCTPStreamParameters `json:"sctpStreamParameters"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			producer, err := peer.ProduceData(payload.Label, payload.Params)
			if err != nil {
				cb(nil, err)
				return
			}
			cb(BuildMessage(struct {
				ProducerID uint `json:"id"`
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
			var payload struct {
				ProducerID uint `json:"producerId"`
				Pause      bool `json:"pause"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			producer, ok := peer.producers[payload.ProducerID]
			if !ok {
				cb(nil, fmt.Errorf("producer not found: %v", payload.ProducerID))
				return
			}
			producer.Pause(payload.Pause)
			cb(nil, nil)
			break
		}
	case Consume:
		{
			var payload struct {
				ProducerID uint `json:"producerId"`
				Paused     bool `json:"paused"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			producer, ok := peer.producers[payload.ProducerID]
			if !ok {
				cb(nil, fmt.Errorf("producer not found: %v", payload.ProducerID))
				return
			}
			consumer, err := peer.Consume(producer, payload.Paused)
			if err != nil {
				cb(nil, err)
				return
			}
			peer.consumers[consumer.Id] = consumer
			resp := struct {
				ConsumerId    string                `json:"id"`
				ProducerId    string                `json:"producerId"`
				Kind          string                `json:"kind"`
				RtpParameters rtc.RTPSendParameters `json:"rtpParameters"`
			}{
				ConsumerId:    strconv.FormatUint(uint64(consumer.Id), 10),
				ProducerId:    strconv.FormatUint(uint64(payload.ProducerID), 10),
				RtpParameters: consumer.GetParameters(),
				Kind:          consumer.Kind().String(),
			}

			cb(BuildMessage(resp))
			break
		}
	case ConsumeData:
		{
			var payload struct {
				ProducerID uint `json:"producerId"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			dc, err := peer.ConsumeData(payload.ProducerID)
			if err != nil {
				cb(nil, err)
				return
			}
			peer.dataConsumers[dc.Id] = dc
			resp := struct {
				ConsumerId          string                   `json:"id"`
				ProducerId          string                   `json:"producerId"`
				Label               string                   `json:"label"`
				SctpStreamParameter rtc.SCTPStreamParameters `json:"sctpStreamParameters"`
			}{
				ConsumerId:          strconv.FormatUint(uint64(dc.Id), 10),
				ProducerId:          strconv.FormatUint(uint64(payload.ProducerID), 10),
				Label:               dc.Label(),
				SctpStreamParameter: dc.GetParameters(),
			}

			cb(BuildMessage(resp))
			break
		}
	case ConsumerResume:
		{
			var payload struct {
				ConsumerID uint `json:"consumerId"`
			}
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err != nil {
				cb(nil, err)
				return
			}
			consumer, ok := peer.consumers[payload.ConsumerID]
			if !ok {
				cb(nil, fmt.Errorf("consumer not found: %v", payload.ConsumerID))
				return
			}
			consumer.Resume()
			cb(nil, nil)
			break
		}
	default:
	}
}
