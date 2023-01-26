package goortc

import (
	"math"

	"github.com/hashicorp/go-multierror"
	"github.com/pion/webrtc/v3"
)

type WebRTCTransport struct {
	id uint

	router           *Router
	iceGatherer      *webrtc.ICEGatherer
	iceParams        webrtc.ICEParameters
	sctpCapabilities webrtc.SCTPCapabilities

	iceConn  *webrtc.ICETransport
	dtlsConn *webrtc.DTLSTransport
	sctpConn *webrtc.SCTPTransport

	currentProducerId uint
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

	return &WebRTCTransport{
		id:          id,
		router:      router,
		iceGatherer: iceGatherer,
		iceConn:     ice,
		dtlsConn:    dtls,
		sctpConn:    sctp,
	}, nil
}

func (t *WebRTCTransport) GetCapabilities() (*TransportCapabilities, error) {
	gatherFinished := make(chan struct{})

	t.iceGatherer.OnLocalCandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			close(gatherFinished)
		}
	})

	// Gather candidates
	err := t.iceGatherer.Gather()
	if err != nil {
		return nil, err
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

	return &TransportCapabilities{
		IceCandidates:    iceCandidates,
		IceParameters:    iceParams,
		DtlsParameters:   dtlsParams,
		SCTPCapabilities: sctpCapabilities,
	}, nil
}

func (t *WebRTCTransport) Connect(dtlsParams webrtc.DTLSParameters) error {
	iceRole := webrtc.ICERoleControlled
	err := t.iceConn.Start(t.iceGatherer, t.iceParams, &iceRole)
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

func (t *WebRTCTransport) Produce(kind webrtc.RTPCodecType, parameters RTPParameters) (*Producer, error) {
	receiver, err := t.router.api.NewRTPReceiver(kind, t.dtlsConn)
	if err != nil {
		return nil, errReceiverNotCreated(err)
	}
	// extParams := GetExtendedParameters(parameters, t.mediaEngine)

	params := GetRTPReceivingParameters(parameters)
	err = receiver.Receive(params)
	receiver.SetRTPParameters(webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{
			parameters.Codecs[0],
		},
	})
	if err != nil {
		return nil, errSettingReceiverParameters(err)
	}
	producer := newProducer(t.getNextProducerId(), receiver, t, parameters)

	err = t.router.AddProducer(producer)
	if err != nil {
		return nil, err
	}

	return producer, nil

}

func (t *WebRTCTransport) getNextProducerId() uint {
	if t.currentProducerId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	t.currentProducerId += 1
	return t.currentProducerId
}
