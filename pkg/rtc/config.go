package rtc

import (
	"os"

	"github.com/itzmanish/go-ortc/v2/pkg/buffer"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
)

type RouterConfig struct {
	transportConfig WebRTCConfig
}
type WebRTCConfig struct {
	me webrtc.MediaEngine
}
type SFUConfig struct {
	bufferFactory *buffer.Factory
	loggerFactory logging.LoggerFactory
	routerConfig  RouterConfig
}

func NewSFUConfig(bff *buffer.Factory) (*SFUConfig, error) {
	me := webrtc.MediaEngine{}
	return &SFUConfig{
		loggerFactory: &logging.DefaultLoggerFactory{
			Writer:          os.Stdout,
			DefaultLogLevel: logging.LogLevelDebug,
			ScopeLevels:     make(map[string]logging.LogLevel),
		},
		bufferFactory: bff,
		routerConfig: RouterConfig{
			transportConfig: WebRTCConfig{
				me: me,
			},
		},
	}, nil
}
