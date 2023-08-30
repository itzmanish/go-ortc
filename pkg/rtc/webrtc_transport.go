package rtc

import (
	"fmt"
	"math"
	"sync"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/dtls"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/ice"

	twccManager "github.com/livekit/mediatransportutil/pkg/twcc"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type WebRTCTransport struct {
	Id uint

	closed    bool
	closeOnce sync.Once
	connected chan bool

	idsMap              map[IDType]uint
	producers           map[uint]*Producer
	consumers           map[uint]*Consumer
	dataProducers       map[uint]*DataProducer
	dataConsumers       map[uint]*DataConsumer
	pendingDataChannels map[uint]*webrtc.DataChannel

	twcc   *twccManager.Responder
	router *Router
	bwe    cc.BandwidthEstimator
	caps   TransportCapabilities

	iceTransport *ice.ICETransport
	dtlsConn     *dtls.DTLSTransport
	sctpConn     *webrtc.SCTPTransport

	Metadata map[string]any
}

type SCTPCapabilities struct {
	webrtc.SCTPCapabilities
	Port          uint `json:"port"`
	IsDataChannel bool `json:"isDataChannel"`
}

type TransportCapabilities struct {
	IceCandidates    []ice.ICECandidate  `json:"iceCandidates"`
	IceParameters    ice.ICEParameters   `json:"iceParameters"`
	DtlsParameters   dtls.DTLSParameters `json:"dtlsParameters"`
	SCTPCapabilities SCTPCapabilities    `json:"sctpParameters"`
}

func newWebRTCTransport(id uint, router *Router, publisher bool) (*WebRTCTransport, error) {
	iceTransport, err := ice.NewICETransport()
	if err != nil {
		return nil, err
	}
	dtlsTransport, err := dtls.NewDTLSTransport(iceTransport, router.bufferFactory)
	if err != nil {
		return nil, err
	}
	t := &WebRTCTransport{
		Id:                  id,
		idsMap:              make(map[IDType]uint),
		producers:           make(map[uint]*Producer),
		consumers:           make(map[uint]*Consumer),
		dataProducers:       make(map[uint]*DataProducer),
		dataConsumers:       make(map[uint]*DataConsumer),
		pendingDataChannels: make(map[uint]*webrtc.DataChannel),
		iceTransport:        iceTransport,
		dtlsConn:            dtlsTransport,
	}
	caps, err := t.generateCapabilitites()
	if err != nil {
		return &WebRTCTransport{}, err
	}
	t.caps = caps
	t.addListeners()
	return t, nil
}

func (t *WebRTCTransport) GetCapabilities() TransportCapabilities {
	return t.caps
}

func (t *WebRTCTransport) IsConnected() bool {
	return t.iceTransport.State() == webrtc.ICETransportStateConnected
}

func (t *WebRTCTransport) Connect(dtlsParams dtls.DTLSParameters, iceParams ice.ICEParameters) error {
	err := t.iceTransport.Start(iceParams)
	if err != nil {
		return err
	}
	return t.dtlsConn.Start(dtlsParams)
}

func (t *WebRTCTransport) Close() {
	if t.closed {
		return
	}
	t.closed = true
	for _, producer := range t.producers {
		producer.Close()
	}
	for _, consumer := range t.consumers {
		consumer.Close()
	}
	t.producers = nil
	t.consumers = nil
	t.Stop()
}

func (t *WebRTCTransport) Stop() {
	t.iceTransport.Stop()
}

func (t *WebRTCTransport) Produce(kind MediaKind, parameters RTPReceiveParameters) (*Producer, error) {
	return nil, nil
}

func (t *WebRTCTransport) ProduceData(label string, parameters SCTPStreamParameters) (*DataProducer, error) {
	_, ok := t.dataProducers[parameters.StreamID]
	if ok {
		return nil, fmt.Errorf("data producer already exists for this stream id")
	}
	dc, ok := t.pendingDataChannels[parameters.StreamID]
	if !ok {
		return nil, fmt.Errorf("data channel doesn't exists for data producer")
	}
	dp := NewDataProducer(t.getNextId(DataProducerID), label, parameters)
	dp.SetDataChannel(dc)
	dp.OnMessage(func(dcm webrtc.DataChannelMessage) {
		t.router.OnDataChannelMessage(dp.Id, dcm)
	})
	t.router.AddDataProducer(dp)
	t.dataProducers[parameters.StreamID] = dp
	return dp, nil
}

func (t *WebRTCTransport) Consume(producerId uint, paused bool) (*Consumer, error) {
	return nil, nil
}

func (t *WebRTCTransport) ConsumeData(dataProducerId uint) (*DataConsumer, error) {
	// dp, ok := t.router.dataProducers[dataProducerId]
	// if !ok {
	// 	return nil, fmt.Errorf("data producer not found, id: %v", dataProducerId)
	// }
	// dataChannel, err := t.api.NewDataChannel(t.sctpConn, &webrtc.DataChannelParameters{
	// 	Label:             dp.label,
	// 	Ordered:           dp.params.Ordered,
	// 	Negotiated:        false,
	// 	MaxPacketLifeTime: dp.channel.MaxPacketLifeTime(),
	// 	MaxRetransmits:    dp.channel.MaxRetransmits(),
	// })
	// if err != nil {
	// 	return nil, err
	// }
	// dc := NewDataConsumer(t.getNextId(DataConsumerID), t.sctpConn.GetCapabilities().MaxMessageSize, dataChannel)
	// if t.IsConnected() {
	// 	dc.SetTranportConnected(true)
	// }
	// t.router.AddDataConsumer(dc)
	// t.dataConsumers[dc.Id] = dc
	return nil, nil
}

func (t *WebRTCTransport) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	return t.dtlsConn.WriteRTCP(pkts)
}

func (t *WebRTCTransport) generateCapabilitites() (TransportCapabilities, error) {
	err := t.iceTransport.GatherCandidates()
	if err != nil {
		return TransportCapabilities{}, err
	}
	iceCandidates, err := t.iceTransport.GetLocalCandidates()
	if err != nil {
		return TransportCapabilities{}, err
	}

	iceParams, err := t.iceTransport.GetLocalParameters()
	if err != nil {
		return TransportCapabilities{}, err
	}

	dtlsParams, err := t.dtlsConn.GetLocalParameters()
	if err != nil {
		return TransportCapabilities{}, err
	}

	// sctpCapabilities := t.sctpConn.GetCapabilities()

	return TransportCapabilities{
		IceCandidates:  iceCandidates,
		IceParameters:  iceParams,
		DtlsParameters: dtlsParams,
		// SCTPCapabilities: SCTPCapabilities{
		// 	SCTPCapabilities: sctpCapabilities,
		// 	Port:             5000,
		// 	IsDataChannel:    true,
		// },
	}, nil
}

func (t *WebRTCTransport) addListeners() {
	t.iceTransport.OnConnectionStateChange(func(is webrtc.ICETransportState) {
		logger.Info("ice state changed", is)
		if is == webrtc.ICETransportStateDisconnected || is == webrtc.ICETransportStateFailed {
			t.closeOnce.Do(func() {
				t.Close()
			})
		}
	})

	t.dtlsConn.OnStateChange(func(ds dtls.DTLSTransportState) {
		logger.Info("dtls state changed", ds)
	})

	// t.sctpConn.OnDataChannel(func(dc *webrtc.DataChannel) {
	// 	logger.Info("data channel found", dc)
	// 	for _, dp := range t.dataProducers {
	// 		if dp.params.StreamID == uint(*dc.ID()) {
	// 			dp.SetDataChannel(dc)
	// 			return
	// 		}
	// 	}
	// 	logger.Warn("no existing data producer found for associated datachannel")
	// 	t.pendingDataChannels[uint(*dc.ID())] = dc
	// })
}

func (t *WebRTCTransport) getNextId(idtype IDType) uint {
	lastID, ok := t.idsMap[idtype]
	switch idtype {
	case RTPProducerID,
		DataProducerID,
		RTPConsumerID,
		DataConsumerID:
		if !ok {
			lastID = 1
		} else {
			if lastID == math.MaxUint {
				panic("Why the hell the id is still uint16!")
			}
			lastID += 1
		}
	case MidID:
		if !ok {
			lastID = 0
		} else {
			lastID += 1
		}
	}
	t.idsMap[idtype] = lastID
	return lastID
}
