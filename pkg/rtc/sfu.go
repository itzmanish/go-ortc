package rtc

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/itzmanish/go-ortc/pkg/logger"
)

type SFU struct {
	PID uint

	config *SFUConfig
	router []*Router

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

	return &SFU{
		PID:    1,
		config: config,
		router: []*Router{},
	}, nil
}

func (sfu *SFU) NewRouter() *Router {
	return NewRouter(uint(sfu.generateNewRouterID()), sfu.config.bufferFactory, sfu.config.routerConfig)
}

func (sfu *SFU) generateNewRouterID() uint16 {
	if sfu.currentRouterId == math.MaxUint16 {
		panic("Why the hell the id is still uint16!")
	}
	sfu.currentRouterId += 1
	return sfu.currentRouterId
}
