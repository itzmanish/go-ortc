package rtc

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/pion/ice/v2"
)

// ICEGatherer gathers local host, server reflexive and relay
// candidates, as well as enabling the retrieval of local Interactive
// Connectivity Establishment (ICE) parameters which can be
// exchanged in signaling.
type ICEGatherer struct {
	lock  sync.RWMutex
	log   logger.Logger
	state ICEGathererState

	validatedServers []*ice.URL
	gatherPolicy     ICETransportPolicy

	agent *ice.Agent

	onLocalCandidateHandler atomic.Value // func(candidate *ICECandidate)
	onStateChangeHandler    atomic.Value // func(state ICEGathererState)

	// Used for GatheringCompletePromise
	onGatheringCompleteHandler atomic.Value // func()
	settingEngine              *SettingEngine
}

// NewICEGatherer creates a new NewICEGatherer.
// This constructor is part of the ORTC  It is not
// meant to be used together with the basic WebRTC
func NewICEGatherer(opts ICEGatherOptions, se *SettingEngine) (*ICEGatherer, error) {
	var validatedServers []*ice.URL
	if len(opts.ICEServers) > 0 {
		for _, server := range opts.ICEServers {
			url, err := server.urls()
			if err != nil {
				return nil, err
			}
			validatedServers = append(validatedServers, url...)
		}
	}

	return &ICEGatherer{
		state:            ICEGathererStateNew,
		gatherPolicy:     opts.ICEGatherPolicy,
		validatedServers: validatedServers,
		settingEngine:    se,
		log:              logger.NewLogger("ice"),
	}, nil
}

func (g *ICEGatherer) createAgent() error {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.agent != nil || g.State() != ICEGathererStateNew {
		return nil
	}

	candidateTypes := []ice.CandidateType{}
	candidateTypes = append(candidateTypes, ice.CandidateTypeHost)

	var nat1To1CandiTyp ice.CandidateType
	switch g.settingEngine.candidates.NAT1To1IPCandidateType {
	case ICECandidateTypeHost:
		nat1To1CandiTyp = ice.CandidateTypeHost
	default:
		nat1To1CandiTyp = ice.CandidateTypeUnspecified
	}

	mDNSMode := g.settingEngine.candidates.MulticastDNSMode
	if mDNSMode != ice.MulticastDNSModeDisabled && mDNSMode != ice.MulticastDNSModeQueryAndGather {
		// If enum is in state we don't recognized default to MulticastDNSModeQueryOnly
		mDNSMode = ice.MulticastDNSModeQueryOnly
	}

	config := &ice.AgentConfig{
		Lite:                   g.settingEngine.candidates.ICELite,
		Urls:                   g.validatedServers,
		PortMin:                g.settingEngine.ephemeralUDP.PortMin,
		PortMax:                g.settingEngine.ephemeralUDP.PortMax,
		DisconnectedTimeout:    g.settingEngine.timeout.ICEDisconnectedTimeout,
		FailedTimeout:          g.settingEngine.timeout.ICEFailedTimeout,
		KeepaliveInterval:      g.settingEngine.timeout.ICEKeepaliveInterval,
		LoggerFactory:          g.settingEngine.LoggerFactory,
		CandidateTypes:         candidateTypes,
		HostAcceptanceMinWait:  g.settingEngine.timeout.ICEHostAcceptanceMinWait,
		SrflxAcceptanceMinWait: g.settingEngine.timeout.ICESrflxAcceptanceMinWait,
		PrflxAcceptanceMinWait: g.settingEngine.timeout.ICEPrflxAcceptanceMinWait,
		RelayAcceptanceMinWait: g.settingEngine.timeout.ICERelayAcceptanceMinWait,
		InterfaceFilter:        g.settingEngine.candidates.InterfaceFilter,
		IPFilter:               g.settingEngine.candidates.IPFilter,
		NAT1To1IPs:             g.settingEngine.candidates.NAT1To1IPs,
		NAT1To1IPCandidateType: nat1To1CandiTyp,
		IncludeLoopback:        g.settingEngine.candidates.IncludeLoopbackCandidate,
		Net:                    g.settingEngine.net,
		MulticastDNSMode:       mDNSMode,
		MulticastDNSHostName:   g.settingEngine.candidates.MulticastDNSHostName,
		LocalUfrag:             g.settingEngine.candidates.UsernameFragment,
		LocalPwd:               g.settingEngine.candidates.Password,
		TCPMux:                 g.settingEngine.iceTCPMux,
		UDPMux:                 g.settingEngine.iceUDPMux,
		ProxyDialer:            g.settingEngine.iceProxyDialer,
	}

	requestedNetworkTypes := g.settingEngine.candidates.ICENetworkTypes
	if len(requestedNetworkTypes) == 0 {
		requestedNetworkTypes = supportedNetworkTypes()
	}

	for _, typ := range requestedNetworkTypes {
		config.NetworkTypes = append(config.NetworkTypes, ice.NetworkType(typ))
	}

	agent, err := ice.NewAgent(config)
	if err != nil {
		return err
	}

	g.agent = agent
	return nil
}

// Gather ICE candidates.
func (g *ICEGatherer) Gather() error {
	if err := g.createAgent(); err != nil {
		return err
	}

	agent := g.getAgent()
	// it is possible agent had just been closed
	if agent == nil {
		return fmt.Errorf("%w: unable to gather", errICEAgentNotExist)
	}

	g.setState(ICEGathererStateGathering)
	if err := agent.OnCandidate(func(candidate ice.Candidate) {
		onLocalCandidateHandler := func(*ICECandidate) {}
		if handler, ok := g.onLocalCandidateHandler.Load().(func(candidate *ICECandidate)); ok && handler != nil {
			onLocalCandidateHandler = handler
		}

		onGatheringCompleteHandler := func() {}
		if handler, ok := g.onGatheringCompleteHandler.Load().(func()); ok && handler != nil {
			onGatheringCompleteHandler = handler
		}

		if candidate != nil {
			c, err := newICECandidateFromICE(candidate)
			if err != nil {
				g.log.Warnf("Failed to convert ice.Candidate: %s", err)
				return
			}
			onLocalCandidateHandler(&c)
		} else {
			g.setState(ICEGathererStateComplete)

			onGatheringCompleteHandler()
			onLocalCandidateHandler(nil)
		}
	}); err != nil {
		return err
	}
	return agent.GatherCandidates()
}

// Close prunes all local candidates, and closes the ports.
func (g *ICEGatherer) Close() error {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.agent == nil {
		return nil
	} else if err := g.agent.Close(); err != nil {
		return err
	}

	g.agent = nil
	g.setState(ICEGathererStateClosed)

	return nil
}

// GetLocalParameters returns the ICE parameters of the ICEGatherer.
func (g *ICEGatherer) GetLocalParameters() (ICEParameters, error) {
	if err := g.createAgent(); err != nil {
		return ICEParameters{}, err
	}

	agent := g.getAgent()
	// it is possible agent had just been closed
	if agent == nil {
		return ICEParameters{}, fmt.Errorf("%w: unable to get local parameters", errICEAgentNotExist)
	}

	frag, pwd, err := agent.GetLocalUserCredentials()
	if err != nil {
		return ICEParameters{}, err
	}

	return ICEParameters{
		UsernameFragment: frag,
		Password:         pwd,
		ICELite:          false,
	}, nil
}

// GetLocalCandidates returns the sequence of valid local candidates associated with the ICEGatherer.
func (g *ICEGatherer) GetLocalCandidates() ([]ICECandidate, error) {
	if err := g.createAgent(); err != nil {
		return nil, err
	}

	agent := g.getAgent()
	// it is possible agent had just been closed
	if agent == nil {
		return nil, fmt.Errorf("%w: unable to get local candidates", errICEAgentNotExist)
	}

	iceCandidates, err := agent.GetLocalCandidates()
	if err != nil {
		return nil, err
	}

	return newICECandidatesFromICE(iceCandidates)
}

// OnLocalCandidate sets an event handler which fires when a new local ICE candidate is available
// Take note that the handler will be called with a nil pointer when gathering is finished.
func (g *ICEGatherer) OnLocalCandidate(f func(*ICECandidate)) {
	g.onLocalCandidateHandler.Store(f)
}

// OnStateChange fires any time the ICEGatherer changes
func (g *ICEGatherer) OnStateChange(f func(ICEGathererState)) {
	g.onStateChangeHandler.Store(f)
}

// State indicates the current state of the ICE gatherer.
func (g *ICEGatherer) State() ICEGathererState {
	return atomicLoadICEGathererState(&g.state)
}

func (g *ICEGatherer) setState(s ICEGathererState) {
	atomicStoreICEGathererState(&g.state, s)

	if handler, ok := g.onStateChangeHandler.Load().(func(state ICEGathererState)); ok && handler != nil {
		handler(s)
	}
}

func (g *ICEGatherer) getAgent() *ice.Agent {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.agent
}
