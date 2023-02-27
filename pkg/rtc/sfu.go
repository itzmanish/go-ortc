package rtc

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

type SFU struct {
	PID uint

	api           *webrtc.API
	config        *SFUConfig
	bufferFactory *buffer.Factory
	router        []*Router

	currentRouterId uint16
}

var (
	packetFactory *sync.Pool
)

func init() {
	// Init packet factory
	packetFactory = &sync.Pool{
		New: func() interface{} {
			b := make([]byte, 1460)
			return &b
		},
	}
	rand.Seed(time.Now().UnixNano())
}

func NewSFU() (*SFU, error) {
	bufferFactory := buffer.NewBufferFactory(500, logger.NewLogger("ORTC Buffer"))
	config, err := NewSFUConfig(bufferFactory)
	if err != nil {
		return nil, err
	}
	ir := &interceptor.Registry{}
	webrtc.RegisterDefaultInterceptors(&config.me, ir)
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(&config.me),
		webrtc.WithSettingEngine(config.se),
		webrtc.WithInterceptorRegistry(ir),
	)
	return &SFU{
		PID:           1,
		config:        config,
		router:        []*Router{},
		api:           api,
		bufferFactory: bufferFactory,
	}, nil
}

func (sfu *SFU) NewRouter() *Router {
	return NewRouter(uint(sfu.generateNewRouterID()), sfu.bufferFactory, &sfu.config.me, sfu.api)
}

func (sfu *SFU) generateNewRouterID() uint16 {
	if sfu.currentRouterId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	sfu.currentRouterId += 1
	return sfu.currentRouterId
}
