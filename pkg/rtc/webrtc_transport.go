package rtc

import (
	"math"

	"github.com/hashicorp/go-multierror"
	"github.com/itzmanish/go-ortc/pkg/logger"

	twccManager "github.com/livekit/mediatransportutil/pkg/twcc"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type WebRTCTransport struct {
	Id uint

	lastProducerId uint
	lastConsumerId uint
	lastMid        uint8
	closed         bool
	connected      chan bool

	twcc             *twccManager.Responder
	router           *Router
	iceGatherer      *webrtc.ICEGatherer
	iceParams        webrtc.ICEParameters
	sctpCapabilities webrtc.SCTPCapabilities
	caps             TransportCapabilities

	me       *webrtc.MediaEngine
	api      *webrtc.API
	iceConn  *webrtc.ICETransport
	dtlsConn *webrtc.DTLSTransport
	sctpConn *webrtc.SCTPTransport

	Metadata map[string]any
}

type TransportCapabilities struct {
	IceCandidates    []webrtc.ICECandidate   `json:"iceCandidates"`
	IceParameters    webrtc.ICEParameters    `json:"iceParameters"`
	DtlsParameters   webrtc.DTLSParameters   `json:"dtlsParameters"`
	SCTPCapabilities webrtc.SCTPCapabilities `json:"sctpCapabilities"`
}

func newWebRTCTransport(id uint, router *Router, publisher bool) (*WebRTCTransport, error) {
	me := &webrtc.MediaEngine{}
	se := router.config.transportConfig.se
	ir := &interceptor.Registry{}

	if true {
		gf, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
			return gcc.NewSendSideBWE(
				gcc.SendSideBWEInitialBitrate(2*1000*1000),
				gcc.SendSideBWEPacer(gcc.NewNoOpPacer()),
			)
		})

		if err == nil {
			gf.OnNewPeerConnection(func(id string, estimator cc.BandwidthEstimator) {
				// if onBandwidthEstimator != nil {
				// 	onBandwidthEstimator(estimator)
				// }
				estimator.OnTargetBitrateChange(func(bitrate int) {
					logger.Info("bitrate changed", bitrate)
				})
			})
			ir.Add(gf)

			tf, err := twcc.NewHeaderExtensionInterceptor()
			if err == nil {
				ir.Add(tf)
			}
		}
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se), webrtc.WithInterceptorRegistry(ir))
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

	ice.OnConnectionStateChange(func(is webrtc.ICETransportState) {
		logger.Info("ice state changed", is)
	})
	ice.OnSelectedCandidatePairChange(func(ip *webrtc.ICECandidatePair) {
		logger.Info("ice selected candidate pair changed", ip)
	})
	dtls.OnStateChange(func(ds webrtc.DTLSTransportState) {
		logger.Info("dtls state changed", ds)
	})
	transport := &WebRTCTransport{
		Id:          id,
		router:      router,
		iceGatherer: iceGatherer,
		iceConn:     ice,
		dtlsConn:    dtls,
		sctpConn:    sctp,
		api:         api,
		me:          me,
		connected:   make(chan bool, 1),
	}

	caps, err := transport.generateCapabilitites()
	if err != nil {
		return nil, err
	}
	transport.caps = caps

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

	// err = t.sctpConn.Start(t.sctpCapabilities)
	// if err != nil {
	// 	multierror.Append(err, t.iceConn.Stop(), t.dtlsConn.Stop())
	// 	return err
	// }

	return nil
}

func (t *WebRTCTransport) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return t.Stop()
}

func (t *WebRTCTransport) Stop() error {
	err := t.iceConn.Stop()
	multierror.Append(err, t.dtlsConn.Stop(), t.sctpConn.Stop())
	return err
}

func (t *WebRTCTransport) Produce(kind MediaKind, parameters RTPReceiveParameters, simulcast bool) (*Producer, error) {
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
	producer := newProducer(t.getNextProducerId(), receiver, t, parameters, simulcast)

	err = t.router.AddProducer(producer)
	if err != nil {
		return nil, err
	}

	go producer.readRTP()
	go producer.handleRTCP()

	return producer, nil

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
	dt, err := NewDownTrack(
		producer.track.Codec().RTPCodecCapability,
		producer, t.router.bufferFactory,
		500,
	)
	if err != nil {
		return nil, errFailedToCreateDownTrack(err)
	}

	consumer, err := newConsumer(t.getNextConsumerId(), producer, dt, t, paused)
	if err != nil {
		return nil, err
	}
	err = t.router.AddConsumer(consumer)
	return consumer, err
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
	t.iceParams = iceParams

	dtlsParams, err := t.dtlsConn.GetLocalParameters()
	if err != nil {
		panic(err)
	}

	sctpCapabilities := t.sctpConn.GetCapabilities()
	t.sctpCapabilities = sctpCapabilities

	return TransportCapabilities{
		IceCandidates:    iceCandidates,
		IceParameters:    iceParams,
		DtlsParameters:   dtlsParams,
		SCTPCapabilities: sctpCapabilities,
	}, nil
}

func (t *WebRTCTransport) getNextProducerId() uint {
	if t.lastProducerId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	t.lastProducerId += 1
	return t.lastProducerId
}

func (t *WebRTCTransport) getNextConsumerId() uint {
	if t.lastConsumerId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	t.lastConsumerId += 1
	return t.lastConsumerId
}

func (t *WebRTCTransport) getMid() uint8 {
	if t.lastMid == math.MaxUint8 {
		panic("are you crazy? How can you create this much of consumers on a transport!")
	}
	defer func() {
		t.lastMid += 1
	}()
	return t.lastMid
}
