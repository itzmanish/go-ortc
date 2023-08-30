package ice

import (
	"context"
	"sync"

	"github.com/itzmanish/go-ortc/v2/internal/mux"
	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/pion/ice/v2"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
)

type ICETransport struct {
	sync.RWMutex
	agent *ice.Agent
	conn  *ice.Conn
	role  ice.Role
	mux   *mux.Mux

	onConnectionStateChange func(webrtc.ICETransportState)
	state                   ice.ConnectionState
	ctxCancel               context.CancelFunc
}

func NewICETransport() (*ICETransport, error) {
	iAgent, err := ice.NewAgent(&ice.AgentConfig{
		Lite:           true,
		PortMin:        40000,
		PortMax:        50000,
		CandidateTypes: []ice.CandidateType{ice.CandidateTypeHost},
		NetworkTypes: []ice.NetworkType{
			ice.NetworkTypeTCP4,
			ice.NetworkTypeUDP4,
		},
	})
	if err != nil {
		return nil, err
	}
	return &ICETransport{
		agent: iAgent,
		role:  ice.Controlled,
	}, nil
}

func (t *ICETransport) Stop() {
	t.ctxCancel()
}

func (t *ICETransport) Start(params ICEParameters) error {
	ctx, cancel := context.WithCancel(context.Background())
	t.ctxCancel = cancel
	t.agent.OnConnectionStateChange(t.updateState)
	conn, err := t.agent.Accept(ctx, params.UsernameFragment, params.Password)
	if err != nil {
		return err
	}
	t.conn = conn
	t.mux = mux.NewMux(mux.Config{
		Conn:          t.conn,
		BufferSize:    1500,
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	return nil
}

func (t *ICETransport) GatherCandidates() error {
	gatheringComplete, done := context.WithCancel(context.Background())
	t.agent.OnCandidate(func(c ice.Candidate) {
		logger.Info("found candidated", c)
		done()
	})
	err := t.agent.GatherCandidates()
	if err != nil {
		return err
	}
	<-gatheringComplete.Done()
	return nil
}

func (t *ICETransport) NewEndpoint(fn mux.MatchFunc) *mux.Endpoint {
	return t.mux.NewEndpoint(fn)
}

func (t *ICETransport) OnConnectionStateChange(fn func(webrtc.ICETransportState)) {
	t.onConnectionStateChange = fn
}

func (t *ICETransport) updateState(state ice.ConnectionState) {
	t.Lock()
	t.state = state
	t.onConnectionStateChange(webrtc.ICETransportState(state))
	t.Unlock()
}

func (t *ICETransport) State() ice.ConnectionState {
	t.RLock()
	defer t.Unlock()
	return t.state
}

func (t *ICETransport) GetLocalCandidates() ([]ICECandidate, error) {
	candidates, err := t.agent.GetLocalCandidates()
	if err != nil {
		return []ICECandidate{}, err
	}
	return newICECandidatesFromICE(candidates)
}

func (t *ICETransport) GetLocalParameters() (ICEParameters, error) {
	user, pwd, err := t.agent.GetLocalUserCredentials()
	if err != nil {
		return ICEParameters{}, err
	}
	return ICEParameters{
		UsernameFragment: user,
		Password:         pwd,
		ICELite:          true,
	}, nil
}
