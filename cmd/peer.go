package main

import (
	"fmt"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/dtls"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/ice"
)

type Peer struct {
	Id            uint
	closed        bool
	router        *rtc.Router
	transports    map[uint]*rtc.WebRTCTransport
	producers     map[uint]*rtc.Producer
	dataProducers map[uint]*rtc.DataProducer
	dataConsumers map[uint]*rtc.DataConsumer
	consumers     map[uint]*rtc.Consumer
}

func NewPeer(id uint, router *rtc.Router) *Peer {
	return &Peer{
		Id:            id,
		router:        router,
		transports:    make(map[uint]*rtc.WebRTCTransport),
		producers:     make(map[uint]*rtc.Producer),
		consumers:     make(map[uint]*rtc.Consumer),
		dataProducers: make(map[uint]*rtc.DataProducer),
		dataConsumers: make(map[uint]*rtc.DataConsumer),
	}
}

func (p *Peer) SetTransport(id uint, transport *rtc.WebRTCTransport) {
	p.transports[id] = transport
}

func (p *Peer) GetTransport(id uint) *rtc.WebRTCTransport {
	return p.transports[id]
}

func (p *Peer) GetConsumingTransport() *rtc.WebRTCTransport {
	for _, t := range p.transports {
		if v, ok := t.Metadata["consuming"]; ok && v.(bool) {
			return t
		}
	}
	return nil
}

func (p *Peer) GetProducingTransport() *rtc.WebRTCTransport {
	for _, t := range p.transports {
		if v, ok := t.Metadata["consuming"]; ok && !v.(bool) {
			return t
		}
	}
	return nil
}

func (p *Peer) Close() {
	p.closed = true
}

func (p *Peer) CreateTransport(consuming bool) (*rtc.WebRTCTransport, error) {
	transport, err := p.router.NewWebRTCTransport(map[string]any{"consuming": consuming})
	if err != nil {
		return nil, err
	}
	p.transports[transport.Id] = transport
	return transport, nil
}

func (p *Peer) ConnectTransport(id uint, dtlsParams dtls.DTLSParameters, iceParams ice.ICEParameters) error {
	transport := p.GetTransport(id)
	if transport == nil {
		return fmt.Errorf("transport not found: %v", id)
	}
	return transport.Connect(dtlsParams, iceParams)
}

func (p *Peer) CloseTransport(id uint) error {
	transport := p.GetTransport(id)
	if transport == nil {
		return fmt.Errorf("transport not found: %v", id)
	}
	transport.Close()
	return nil
}

func (p *Peer) Produce(kind string, parameters rtc.RTPReceiveParameters) (*rtc.Producer, error) {
	transport := p.GetProducingTransport()
	if transport == nil {
		return nil, fmt.Errorf("producing transport not found")
	}
	var mediaKind rtc.MediaKind
	if kind == "video" {
		mediaKind = rtc.VideoMediaKind
	} else if kind == "audio" {
		mediaKind = rtc.AudioMediaKind
	} else {
		return nil, fmt.Errorf("unknown media kind to produce: %v", kind)
	}
	producer, err := transport.Produce(mediaKind, parameters)
	if err != nil {
		return nil, err
	}
	p.producers[producer.Id] = producer
	return producer, err
}

func (p *Peer) Consume(producer *rtc.Producer, paused bool) (*rtc.Consumer, error) {
	logger.Info("Consuming producerID", producer.Id, "paused:", paused)
	transport := p.GetConsumingTransport()
	if transport == nil {
		return nil, fmt.Errorf("consuming transport not found")
	}
	return transport.Consume(producer.Id, paused)
}

func (p *Peer) ProduceData(label string, params rtc.SCTPStreamParameters) (*rtc.DataProducer, error) {
	logger.Info("Producing data", label, "params", params)
	transport := p.GetProducingTransport()
	if transport == nil {
		return nil, fmt.Errorf("producing transport not found")
	}
	return transport.ProduceData(label, params)
}

func (p *Peer) ConsumeData(id uint) (*rtc.DataConsumer, error) {
	logger.Info("Consuming data producerID", id)
	transport := p.GetConsumingTransport()
	if transport == nil {
		return nil, fmt.Errorf("consuming transport not found")
	}
	return transport.ConsumeData(id)
}
