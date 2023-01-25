package goortc

import (
	"github.com/hashicorp/go-multierror"
	"github.com/pion/webrtc/v3"
)

type WebRTCTransport struct {
	id uint

	api              *webrtc.API
	iceGatherer      *webrtc.ICEGatherer
	iceParams        webrtc.ICEParameters
	sctpCapabilities webrtc.SCTPCapabilities

	iceConn  *webrtc.ICETransport
	dtlsConn *webrtc.DTLSTransport
	sctpConn *webrtc.SCTPTransport
}

type TransportCapabilities struct {
	IceCandidates    []webrtc.ICECandidate   `json:"ice_candidates"`
	IceParameters    webrtc.ICEParameters    `json:"ice_parameters"`
	DtlsParameters   webrtc.DTLSParameters   `json:"dtls_parameters"`
	SCTPCapabilities webrtc.SCTPCapabilities `json:"sctp_capabilities"`
}

func NewWebRTCTransport(id uint, api *webrtc.API) (*WebRTCTransport, error) {
	iceGatherer, err := api.NewICEGatherer(webrtc.ICEGatherOptions{})
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
		api:         api,
		iceGatherer: iceGatherer,
		iceConn:     ice,
		dtlsConn:    dtls,
		sctpConn:    sctp,
	}, nil
}

func (t *WebRTCTransport) getCapabilities() (*TransportCapabilities, error) {
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
	err := t.iceConn.Start(nil, t.iceParams, &iceRole)
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
