package rtc

import (
	"fmt"
	"math"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/itzmanish/go-ortc/pkg/logger"

	twccManager "github.com/livekit/mediatransportutil/pkg/twcc"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
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

	twcc        *twccManager.Responder
	router      *Router
	iceGatherer *webrtc.ICEGatherer
	bwe         cc.BandwidthEstimator
	caps        TransportCapabilities

	me       *webrtc.MediaEngine
	api      *webrtc.API
	iceConn  *webrtc.ICETransport
	dtlsConn *webrtc.DTLSTransport
	sctpConn *webrtc.SCTPTransport

	Metadata map[string]any
}

type SCTPCapabilities struct {
	webrtc.SCTPCapabilities
	Port          uint `json:"port"`
	IsDataChannel bool `json:"isDataChannel"`
}

type TransportCapabilities struct {
	IceCandidates    []webrtc.ICECandidate `json:"iceCandidates"`
	IceParameters    webrtc.ICEParameters  `json:"iceParameters"`
	DtlsParameters   webrtc.DTLSParameters `json:"dtlsParameters"`
	SCTPCapabilities SCTPCapabilities      `json:"sctpParameters"`
}

func newWebRTCTransport(id uint, router *Router, publisher bool) (*WebRTCTransport, error) {
	me := &webrtc.MediaEngine{}
	se := router.config.transportConfig.se

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se))
	iceGatherer, err := api.NewICEGatherer(webrtc.ICEGatherOptions{
		ICEGatherPolicy: webrtc.ICETransportPolicyAll,
	})
	if err != nil {
		return nil, err
	}
	ice := api.NewICETransport(iceGatherer)
	dtls, err := api.NewDTLSTransport(ice, nil)
	if err != nil {
		return nil, err
	}

	sctp := api.NewSCTPTransport(dtls)

	transport := &WebRTCTransport{
		Id:                  id,
		router:              router,
		iceGatherer:         iceGatherer,
		iceConn:             ice,
		dtlsConn:            dtls,
		sctpConn:            sctp,
		api:                 api,
		me:                  me,
		closeOnce:           sync.Once{},
		connected:           make(chan bool, 1),
		producers:           make(map[uint]*Producer),
		consumers:           make(map[uint]*Consumer),
		dataProducers:       make(map[uint]*DataProducer),
		dataConsumers:       make(map[uint]*DataConsumer),
		pendingDataChannels: make(map[uint]*webrtc.DataChannel),
		idsMap:              make(map[IDType]uint),
	}

	caps, err := transport.generateCapabilitites()
	if err != nil {
		return nil, err
	}
	transport.caps = caps
	transport.addListeners()

	return transport, nil
}

func (t *WebRTCTransport) GetCapabilities() TransportCapabilities {
	return t.caps
}

func (t *WebRTCTransport) IsConnected() bool {
	return t.iceConn.State() == webrtc.ICETransportStateConnected
}

func (t *WebRTCTransport) Connect(dtlsParams webrtc.DTLSParameters, iceParams webrtc.ICEParameters) error {
	iceRole := webrtc.ICERoleControlled

	err := t.iceConn.Start(nil, iceParams, &iceRole)
	if err != nil {
		return err
	}

	err = t.dtlsConn.Start(dtlsParams)
	if err != nil {
		multierror.Append(err, t.iceConn.Stop())
		return err
	}

	err = t.sctpConn.Start(webrtc.SCTPCapabilities{})
	if err != nil {
		multierror.Append(err, t.iceConn.Stop(), t.dtlsConn.Stop())
		return err
	}

	return nil
}

func (t *WebRTCTransport) Close() error {
	if t.closed {
		return nil
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
	return t.Stop()
}

func (t *WebRTCTransport) Stop() error {
	err := t.iceConn.Stop()
	multierror.Append(err, t.dtlsConn.Stop(), t.sctpConn.Stop())
	return err
}

func (t *WebRTCTransport) Produce(kind MediaKind, parameters RTPReceiveParameters) (*Producer, error) {
	err := SetupProducerMediaEngineForKind(t.me, kind)
	if err != nil {
		return nil, err
	}
	receiver, err := t.api.NewRTPReceiver(webrtc.RTPCodecType(kind), t.dtlsConn)
	if err != nil {
		return nil, errReceiverNotCreated(err)
	}

	params := ParseRTPReciveParametersFromORTC(parameters)
	err = receiver.Receive(params)
	if err != nil {
		return nil, errSettingReceiverParameters(err)
	}
	validParams := ParseRTPParametersFromORTC(RTPParameters{
		HeaderExtensions: parameters.HeaderExtensions,
		Codecs:           parameters.Codecs,
		Mid:              parameters.Mid,
		Rtcp:             parameters.Rtcp,
	})
	receiver.SetRTPParameters(RemoveRTXCodecsFromRTPParameters(validParams))
	if t.twcc == nil {
		ssrc := receiver.Track().SSRC()
		t.twcc = twccManager.NewTransportWideCCResponder(uint32(ssrc))
		t.twcc.OnFeedback(func(pkt rtcp.RawPacket) {
			logger.Debugf("TRANSPORT::TWCC::OnFeedback() sending packet: %+v", pkt)
			_, err := t.WriteRTCP([]rtcp.Packet{&pkt})
			if err != nil {
				logger.Error("error on writing TWCC feedback packet", err)
			}
		})
	}
	producer := newProducer(t.getNextId(RTPProducerID), receiver, t, parameters)

	err = t.router.AddProducer(producer)
	if err != nil {
		return nil, err
	}

	go producer.readRTPWorker()

	t.producers[producer.Id] = producer

	return producer, nil
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
	producer, ok := t.router.producers[producerId]
	if !ok {
		return nil, errProducerNotFound(producerId)
	}
	err := SetupConsumerMediaEngineWithProducerParams(t.me,
		ParseRTPParametersFromORTC(ConvertRTPRecieveParametersToRTPParamters(producer.parameters)),
		webrtc.RTPCodecType(producer.kind),
	)
	if err != nil {
		return nil, err
	}
	if t.bwe == nil {
		bwe, err := gcc.NewSendSideBWE(
			gcc.SendSideBWEInitialBitrate(2*1000*1000),
			gcc.SendSideBWEPacer(gcc.NewNoOpPacer()),
		)
		if err != nil {
			return nil, err
		}
		bwe.OnTargetBitrateChange(func(bitrate int) {
			logger.Info("bitrate changed", bitrate)
		})
		t.bwe = bwe
	}
	dt, err := NewDownTrack(
		t.getNextId(MidID),
		producer.track.Codec().RTPCodecCapability,
		producer, t.router.bufferFactory,
		500,
	)
	if err != nil {
		return nil, errFailedToCreateDownTrack(err)
	}
	dt.OnTransportCCFeedback(func(tlc *rtcp.TransportLayerCC) {
		logger.Debug("sending transport cc to evaluate", tlc)
		err := t.bwe.WriteRTCP([]rtcp.Packet{tlc}, nil)
		if err != nil {
			logger.Error("Error on processing incoming tcc packet", err)
		}
	})
	consumer, err := newConsumer(t.getNextId(RTPConsumerID), producer, dt, t, paused)
	if err != nil {
		return nil, err
	}
	err = t.router.AddConsumer(consumer)
	t.consumers[consumer.Id] = consumer
	dt.OnCloseHandler(func() {
		consumer.Close()
	})
	return consumer, err
}

func (t *WebRTCTransport) ConsumeData(dataProducerId uint) (*DataConsumer, error) {
	dp, ok := t.router.dataProducers[dataProducerId]
	if !ok {
		return nil, fmt.Errorf("data producer not found, id: %v", dataProducerId)
	}
	dataChannel, err := t.api.NewDataChannel(t.sctpConn, &webrtc.DataChannelParameters{
		Label:             dp.label,
		Ordered:           dp.params.Ordered,
		Negotiated:        false,
		MaxPacketLifeTime: dp.channel.MaxPacketLifeTime(),
		MaxRetransmits:    dp.channel.MaxRetransmits(),
	})
	if err != nil {
		return nil, err
	}
	dc := NewDataConsumer(t.getNextId(DataConsumerID), t.sctpConn.GetCapabilities().MaxMessageSize, dataChannel)
	if t.IsConnected() {
		dc.SetTranportConnected(true)
	}
	t.router.AddDataConsumer(dc)
	t.dataConsumers[dc.Id] = dc
	return dc, nil
}

func (t *WebRTCTransport) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	return t.dtlsConn.WriteRTCP(pkts)
}

func (t *WebRTCTransport) generateCapabilitites() (TransportCapabilities, error) {
	gatherFinished := make(chan struct{})

	t.iceGatherer.OnLocalCandidate(func(i *webrtc.ICECandidate) {
		logger.Info("ice gatherer local candidate found", i)
		if i == nil {
			close(gatherFinished)
		}
	})

	// Gather candidates
	err := t.iceGatherer.Gather()
	if err != nil {
		return TransportCapabilities{}, err
	}

	<-gatherFinished

	iceCandidates, err := t.iceGatherer.GetLocalCandidates()
	if err != nil {
		panic(err)
	}

	iceParams, err := t.iceGatherer.GetLocalParameters()
	if err != nil {
		panic(err)
	}
	// pion webrtc is not using agent's ice-lite settings on the
	// GetLocalParameters() response. So I am manully overriding it here
	iceParams.ICELite = true

	dtlsParams, err := t.dtlsConn.GetLocalParameters()
	if err != nil {
		panic(err)
	}

	sctpCapabilities := t.sctpConn.GetCapabilities()

	return TransportCapabilities{
		IceCandidates:  iceCandidates,
		IceParameters:  iceParams,
		DtlsParameters: dtlsParams,
		SCTPCapabilities: SCTPCapabilities{
			SCTPCapabilities: sctpCapabilities,
			Port:             5000,
			IsDataChannel:    true,
		},
	}, nil
}

func (t *WebRTCTransport) addListeners() {
	t.iceConn.OnConnectionStateChange(func(is webrtc.ICETransportState) {
		logger.Info("ice state changed", is)
		if is == webrtc.ICETransportStateDisconnected || is == webrtc.ICETransportStateFailed {
			t.closeOnce.Do(func() {
				t.Close()
			})
		}
	})
	t.iceConn.OnSelectedCandidatePairChange(func(ip *webrtc.ICECandidatePair) {
		logger.Info("ice selected candidate pair changed", ip)
	})
	t.dtlsConn.OnStateChange(func(ds webrtc.DTLSTransportState) {
		logger.Info("dtls state changed", ds)
	})

	t.sctpConn.OnDataChannel(func(dc *webrtc.DataChannel) {
		logger.Info("data channel found", dc)
		for _, dp := range t.dataProducers {
			if dp.params.StreamID == uint(*dc.ID()) {
				dp.SetDataChannel(dc)
				return
			}
		}
		logger.Warn("no existing data producer found for associated datachannel")
		t.pendingDataChannels[uint(*dc.ID())] = dc
	})
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
