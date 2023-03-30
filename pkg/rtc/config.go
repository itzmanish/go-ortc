package rtc

import (
	"os"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
)

type RouterConfig struct {
	transportConfig WebRTCConfig
}
type WebRTCConfig struct {
	me webrtc.MediaEngine
	se webrtc.SettingEngine
}
type SFUConfig struct {
	bufferFactory *buffer.Factory
	routerConfig  RouterConfig
}

func NewSFUConfig(bff *buffer.Factory) (*SFUConfig, error) {
	me := webrtc.MediaEngine{}
	se := webrtc.SettingEngine{
		LoggerFactory: &logging.DefaultLoggerFactory{
			Writer:          os.Stdout,
			DefaultLogLevel: logging.LogLevelDebug,
			ScopeLevels:     make(map[string]logging.LogLevel),
		},
		BufferFactory: bff.GetOrNew,
	}
	err := se.SetEphemeralUDPPortRange(40000, 50000)
	if err != nil {
		return nil, err
	}
	se.DisableMediaEngineCopy(true)
	se.SetLite(true)
	// FIXME: Only for debugging
	// se.SetICETimeouts(5*time.Minute, 5*time.Minute, 5*time.Minute)
	return &SFUConfig{
		bufferFactory: bff,
		routerConfig: RouterConfig{
			transportConfig: WebRTCConfig{
				me: me,
				se: se,
			},
		},
	}, nil
}
