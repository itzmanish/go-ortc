package rtc

import (
	"fmt"
	"math"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/itzmanish/go-ortc/pkg/saver"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"go.uber.org/atomic"
)

// A router object is equivalent to a room
// all the data for same room can be forwarded to associated
// to and from consumer and producer respectively.
type Router struct {
	Id uint
	logger.Logger

	closed                     atomic.Bool
	transports                 map[uint]*WebRTCTransport
	producers                  map[uint]*Producer
	consumers                  map[uint]*Consumer
	dataProducers              map[uint]*DataProducer
	dataConsumers              map[uint]*DataConsumer
	producerIdToConsumerIdsMap map[uint][]uint
	bufferFactory              *buffer.Factory
	rtcpCh                     chan []rtcp.Packet
	capabilities               RTPCapabilities
	config                     RouterConfig

	currentTransportId uint
}

func NewRouter(id uint, bff *buffer.Factory, config RouterConfig) *Router {
	return &Router{
		Id:                         id,
		config:                     config,
		rtcpCh:                     make(chan []rtcp.Packet),
		transports:                 map[uint]*WebRTCTransport{},
		producers:                  map[uint]*Producer{},
		consumers:                  map[uint]*Consumer{},
		dataProducers:              map[uint]*DataProducer{},
		dataConsumers:              map[uint]*DataConsumer{},
		producerIdToConsumerIdsMap: map[uint][]uint{},
		currentTransportId:         0,
		bufferFactory:              bff,
		capabilities:               DefaultRouterCapabilities(),
		Logger:                     logger.NewLogger("Router").WithField("ID", id),
	}
}

func (router *Router) Close() {
	if router.closed.Swap(true) {
		return
	}
	for _, t := range router.transports {
		t.Close()
	}
	router.transports = nil
	router.producerIdToConsumerIdsMap = nil
	router.consumers = nil
	router.producers = nil
}

func (router *Router) GetRouterCapabilities() RTPCapabilities {
	return router.capabilities
}

func (router *Router) NewWebRTCTransport(metadata map[string]any) (*WebRTCTransport, error) {
	transport, err := newWebRTCTransport(router.generateNewWebrtcTransportID(), router, false)
	if err != nil {
		return nil, err
	}
	transport.Metadata = metadata
	router.transports[transport.Id] = transport
	return transport, nil
}

func (router *Router) AddProducer(producer *Producer) error {
	track := producer.receiver.Track()
	buff, rtcpReader := router.bufferFactory.GetBufferPair(uint32(track.SSRC()))
	if buff == nil || rtcpReader == nil {
		return fmt.Errorf("router.AddProducer(): buff is nil")
	}
	buff.OnRtcpFeedback(producer.SendRTCP)
	rtcpReader.OnPacket(producer.handleRTCP)
	buff.SetTWCC(producer.twcc)

	buff.Bind(ParseRTPParametersFromORTC(ConvertRTPRecieveParametersToRTPParamters(producer.parameters)))

	producer.buffer = buff
	producer.OnRTP(router.OnRTPPacket())
	producer.OnRTCP(router.OnRTCPPacket())
	router.producers[producer.Id] = producer
	router.producerIdToConsumerIdsMap[producer.Id] = []uint{}
	return nil
}

func (router *Router) AddConsumer(consumer *Consumer) error {
	router.consumers[consumer.Id] = consumer
	consumerIds, ok := router.producerIdToConsumerIdsMap[consumer.producer.Id]
	if !ok {
		return fmt.Errorf("associated producer entry not found in producerIdToConsumerIdsMap: %v", consumer.producer.Id)
	}
	consumerIds = append(consumerIds, consumer.Id)
	router.producerIdToConsumerIdsMap[consumer.producer.Id] = consumerIds

	return nil
}

func (router *Router) AddDataProducer(dp *DataProducer) {
	router.dataProducers[dp.Id] = dp
}

func (router *Router) AddDataConsumer(dc *DataConsumer) {
	router.dataConsumers[dc.Id] = dc
}

func (router *Router) OnRTPPacket() OnRTPPacketHandlerFunc {
	return func(producerId uint, rtp *buffer.ExtPacket) {
		// router.Logger.Info("packet found now need to forward", producerId, rtp)
		// here get the associated consumers for the producer id
		consumersId, ok := router.producerIdToConsumerIdsMap[producerId]
		if !ok {
			router.Logger.Warn("how come this producer entry is not in the map", producerId)
			return
		}
		for _, cId := range consumersId {
			consumer, ok := router.consumers[cId]
			if !ok {
				router.Logger.Warn("consumer id not in the map", cId)
				return
			}
			err := consumer.Write(rtp)
			if err != nil {
				router.Logger.Error("Writing rtp packet err:", err)
			}
		}
	}
}

func (router *Router) OnRTCPPacket() OnRTCPPacketHandlerFunc {
	return func(producerId uint, rtcp []rtcp.Packet) {
		router.Logger.Info("rtcp packet found on producer", producerId, rtcp)
		// if required forward or do any operation on the packet
	}
}

func (router *Router) OnDataChannelMessage(dataProducerId uint, message webrtc.DataChannelMessage) {
	router.Logger.Info("got dc message", dataProducerId, message)
	for _, dc := range router.dataConsumers {
		err := dc.Send(message.Data)
		if err != nil {
			router.Logger.Error("unable to forward data channel message to", dc.Id)
		}
	}
}

func (router *Router) generateNewWebrtcTransportID() uint {
	if router.currentTransportId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	router.currentTransportId += 1
	return router.currentTransportId
}

// NOTE: debugging purpose only
func (router *Router) storeConsumerData(filename string, consumer *Consumer) {
	saver := saver.NewWebmSaver(filename)
	saver.InitWriter(0, 0)
	consumer.onRTPPacket = func(id uint, packet *buffer.ExtPacket) {
		if consumer.Kind() == AudioMediaKind {
			saver.PushOpus(packet.Packet.Clone())
		} else if consumer.Kind() == VideoMediaKind {
			saver.PushVP8(packet.Packet.Clone())
		}
	}
}
