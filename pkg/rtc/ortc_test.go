package rtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itzmanish/go-ortc/v2/pkg/buffer"
	"github.com/livekit/mediatransportutil/pkg/bucket"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/stretchr/testify/assert"
)

var tempBuff = buffer.NewFactoryOfBufferFactory(35).CreateBufferFactory()

func Test_ORTC_Media(t *testing.T) {

	stackA, stackB, err := newORTCPair()
	assert.NoError(t, err)

	assert.NoError(t, signalORTCPair(stackA, stackB))

	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	assert.NoError(t, err)

	rtpSender, err := stackA.api.NewRTPSender(track, stackA.dtls)
	assert.NoError(t, err)
	sendingParams := rtpSender.GetParameters()
	b, _ := json.Marshal(sendingParams)
	fmt.Println(string(b))
	fmt.Println("send params:", sendingParams.Encodings)
	assert.NoError(t, rtpSender.Send(sendingParams))

	rtpReceiver, err := stackB.api.NewRTPReceiver(webrtc.RTPCodecTypeVideo, stackB.dtls)
	assert.NoError(t, err)
	assert.NoError(t, rtpReceiver.Receive(webrtc.RTPReceiveParameters{Encodings: []webrtc.RTPDecodingParameters{
		{RTPCodingParameters: rtpSender.GetParameters().Encodings[0].RTPCodingParameters},
	}}))

	seenPacket, seenPacketCancel := context.WithCancel(context.Background())
	go func() {
		track := rtpReceiver.Track()
		buff := tempBuff.GetBuffer(uint32(track.SSRC()))
		if buff == nil {
			t.Error("buffer not found")
		}
		buff.Bind(rtpReceiver.GetParameters())
		pktBuf := make([]byte, bucket.MaxPktSize)
		extP, err := buff.ReadExtended(pktBuf)
		// p, _, err := track.ReadRTP()
		fmt.Println("packet:", extP)
		fmt.Print("payload", extP.Payload)
		assert.NoError(t, err)

		seenPacketCancel()
	}()

	func() {
		for range time.Tick(time.Millisecond * 20) {
			select {
			case <-seenPacket.Done():
				return
			default:
				assert.NoError(t, track.WriteSample(media.Sample{Data: []byte{0xAA}, Duration: time.Second}))
			}
		}
	}()

	assert.NoError(t, rtpSender.Stop())
	assert.NoError(t, rtpReceiver.Stop())

	assert.NoError(t, stackA.close())
	assert.NoError(t, stackB.close())
}

type testORTCStack struct {
	api      *webrtc.API
	gatherer *webrtc.ICEGatherer
	ice      *webrtc.ICETransport
	dtls     *webrtc.DTLSTransport
	sctp     *webrtc.SCTPTransport
}

func (s *testORTCStack) setSignal(sig *testORTCSignal, isOffer bool) error {
	iceRole := webrtc.ICERoleControlled
	if isOffer {
		iceRole = webrtc.ICERoleControlling
	}

	err := s.ice.SetRemoteCandidates(sig.ICECandidates)
	if err != nil {
		return err
	}

	// Start the ICE transport
	err = s.ice.Start(nil, sig.ICEParameters, &iceRole)
	if err != nil {
		return err
	}

	// Start the DTLS transport
	err = s.dtls.Start(sig.DTLSParameters)
	if err != nil {
		return err
	}

	// Start the SCTP transport
	err = s.sctp.Start(sig.SCTPCapabilities)
	if err != nil {
		return err
	}

	return nil
}

func (s *testORTCStack) getSignal() (*testORTCSignal, error) {
	gatherFinished := make(chan struct{})
	s.gatherer.OnLocalCandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			close(gatherFinished)
		}
	})

	if err := s.gatherer.Gather(); err != nil {
		return nil, err
	}

	<-gatherFinished
	iceCandidates, err := s.gatherer.GetLocalCandidates()
	if err != nil {
		return nil, err
	}

	iceParams, err := s.gatherer.GetLocalParameters()
	if err != nil {
		return nil, err
	}

	dtlsParams, err := s.dtls.GetLocalParameters()
	if err != nil {
		return nil, err
	}

	sctpCapabilities := s.sctp.GetCapabilities()

	return &testORTCSignal{
		ICECandidates:    iceCandidates,
		ICEParameters:    iceParams,
		DTLSParameters:   dtlsParams,
		SCTPCapabilities: sctpCapabilities,
	}, nil
}

func (s *testORTCStack) close() error {
	var closeErrs []error

	if err := s.sctp.Stop(); err != nil {
		closeErrs = append(closeErrs, err)
	}

	if err := s.ice.Stop(); err != nil {
		closeErrs = append(closeErrs, err)
	}

	return FlattenErrs(closeErrs)
}

type testORTCSignal struct {
	ICECandidates    []webrtc.ICECandidate
	ICEParameters    webrtc.ICEParameters
	DTLSParameters   webrtc.DTLSParameters
	SCTPCapabilities webrtc.SCTPCapabilities
}

func newORTCPair() (stackA *testORTCStack, stackB *testORTCStack, err error) {
	sa, err := newORTCStack(false)
	if err != nil {
		return nil, nil, err
	}

	sb, err := newORTCStack(true)
	if err != nil {
		return nil, nil, err
	}

	return sa, sb, nil
}

func newORTCStack(useBuffer bool) (*testORTCStack, error) {
	m := &webrtc.MediaEngine{}
	err := m.RegisterDefaultCodecs()
	if err != nil {
		return nil, err
	}
	se := webrtc.SettingEngine{}
	se.DisableMediaEngineCopy(true)
	if useBuffer {
		se.BufferFactory = tempBuff.GetOrNew
	}
	// Create an API object
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se))

	// Create the ICE gatherer
	gatherer, err := api.NewICEGatherer(webrtc.ICEGatherOptions{})
	if err != nil {
		return nil, err
	}

	// Construct the ICE transport
	ice := api.NewICETransport(gatherer)

	// Construct the DTLS transport
	dtls, err := api.NewDTLSTransport(ice, nil)
	if err != nil {
		return nil, err
	}

	// Construct the SCTP transport
	sctp := api.NewSCTPTransport(dtls)

	return &testORTCStack{
		api:      api,
		gatherer: gatherer,
		ice:      ice,
		dtls:     dtls,
		sctp:     sctp,
	}, nil
}

func signalORTCPair(stackA *testORTCStack, stackB *testORTCStack) error {
	sigA, err := stackA.getSignal()
	if err != nil {
		return err
	}
	sigB, err := stackB.getSignal()
	if err != nil {
		return err
	}

	a := make(chan error)
	b := make(chan error)

	go func() {
		a <- stackB.setSignal(sigA, false)
	}()

	go func() {
		b <- stackA.setSignal(sigB, true)
	}()

	errA := <-a
	errB := <-b

	closeErrs := []error{errA, errB}

	return FlattenErrs(closeErrs)
}

// FlattenErrs flattens multiple errors into one
func FlattenErrs(errs []error) error {
	errs2 := []error{}
	for _, e := range errs {
		if e != nil {
			errs2 = append(errs2, e)
		}
	}
	if len(errs2) == 0 {
		return nil
	}
	return multiError(errs2)
}

type multiError []error //nolint:errname

func (me multiError) Error() string {
	var errstrings []string

	for _, err := range me {
		if err != nil {
			errstrings = append(errstrings, err.Error())
		}
	}

	if len(errstrings) == 0 {
		return "multiError must contain multiple error but is empty"
	}

	return strings.Join(errstrings, "\n")
}

func (me multiError) Is(err error) bool {
	for _, e := range me {
		if errors.Is(e, err) {
			return true
		}
		if me2, ok := e.(multiError); ok { //nolint:errorlint
			if me2.Is(err) {
				return true
			}
		}
	}
	return false
}
