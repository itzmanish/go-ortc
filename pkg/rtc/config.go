package rtc

import (
	"os"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
)

type SFUConfig struct {
	me webrtc.MediaEngine
	se webrtc.SettingEngine
}

func NewSFUConfig(bff *buffer.Factory) (*SFUConfig, error) {
	me := webrtc.MediaEngine{}
	err := me.RegisterDefaultCodecs()
	if err != nil {
		return nil, err
	}
	se := webrtc.SettingEngine{
		LoggerFactory: &logging.DefaultLoggerFactory{
			Writer:          os.Stdout,
			DefaultLogLevel: logging.LogLevelDebug,
			ScopeLevels:     make(map[string]logging.LogLevel),
		},
		BufferFactory: bff.GetOrNew,
	}
	// udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
	// 	IP:   net.IP{0, 0, 0, 0},
	// 	Port: 5000,
	// })
	// if err != nil {
	// 	panic(err)
	// }
	// se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))
	se.SetLite(true)
	return &SFUConfig{
		me: me,
		se: se,
	}, nil
}
