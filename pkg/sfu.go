package goortc

import (
	"math"

	"github.com/pion/webrtc/v3"
)

type SFU struct {
	PID uint

	api    *webrtc.API
	config *SFUConfig
	router []*Router

	currentRouterId uint16
}

func NewSFU() (*SFU, error) {
	config, err := NewSFUConfig()
	if err != nil {
		return nil, err
	}
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(&config.me),
		webrtc.WithSettingEngine(config.se),
	)
	return &SFU{
		PID:    1,
		config: config,
		router: []*Router{},
		api:    api,
	}, nil
}

func (sfu *SFU) NewRouter() *Router {
	return NewRouter(uint(sfu.generateNewRouterID()), &sfu.config.me, sfu.api)
}

func (sfu *SFU) generateNewRouterID() uint16 {
	if sfu.currentRouterId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	sfu.currentRouterId += 1
	return sfu.currentRouterId
}

func (sfu *SFU) blah() {

}
