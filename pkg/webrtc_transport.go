package goortc

import (
	"math"

	"github.com/hashicorp/go-multierror"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/webrtc/v3"
)

type WebRTCTransport struct {
	id uint

	router           *Router
	iceGatherer      *webrtc.ICEGatherer
	iceParams        webrtc.ICEParameters
	sctpCapabilities webrtc.SCTPCapabilities
	caps             TransportCapabilities

	iceConn  *webrtc.ICETransport
	dtlsConn *webrtc.DTLSTransport
	sctpConn *webrtc.SCTPTransport

	lastProducerId uint
	lastConsumerId uint
	lastMid        uint8
}

type TransportCapabilities struct {
	IceCandidates    []webrtc.ICECandidate   `json:"ice_candidates"`
	IceParameters    webrtc.ICEParameters    `json:"ice_parameters"`
	DtlsParameters   webrtc.DTLSParameters   `json:"dtls_parameters"`
	SCTPCapabilities webrtc.SCTPCapabilities `json:"sctp_capabilities"`
}

func newWebRTCTransport(id uint, router *Router) (*WebRTCTransport, error) {
	api := router.api
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
		id:          id,
		router:      router,
		iceGatherer: iceGatherer,
		iceConn:     ice,
		dtlsConn:    dtls,
		sctpConn:    sctp,
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

	err = t.sctpConn.Start(t.sctpCapabilities)
	if err != nil {
		multierror.Append(err, t.iceConn.Stop(), t.dtlsConn.Stop())
		return err
	}
	return nil
}

func (t *WebRTCTransport) Stop() error {
	err := t.iceConn.Stop()
	multierror.Append(err, t.dtlsConn.Stop(), t.sctpConn.Stop())
	return err
}

func (t *WebRTCTransport) Produce(kind webrtc.RTPCodecType, parameters RTPParameters, simulcast bool) (*Producer, error) {
	receiver, err := t.router.api.NewRTPReceiver(kind, t.dtlsConn)
	if err != nil {
		return nil, errReceiverNotCreated(err)
	}
	// extParams := GetExtendedParameters(parameters, t.mediaEngine)

	params := GetRTPReceivingParameters(parameters)
	err = receiver.Receive(params)
	if err != nil {
		return nil, errSettingReceiverParameters(err)
	}

	receiver.SetRTPParameters(webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{
			parameters.Codecs[0],
		},
	})
	producer := newProducer(t.getNextProducerId(), receiver, t, parameters, simulcast)

	err = t.router.AddProducer(producer)
	if err != nil {
		return nil, err
	}

	return producer, nil

}

func (t *WebRTCTransport) Consume(producerId uint, paused bool) (*Consumer, error) {
	producer, ok := t.router.producers[producerId]
	if !ok {
		return nil, errProducerNotFound(producerId)
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
	t.lastMid += 1
	return t.lastMid
}
