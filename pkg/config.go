package goortc

import (
	"net"

	"github.com/pion/webrtc/v3"
)

type SFUConfig struct {
	me webrtc.MediaEngine
	se webrtc.SettingEngine
}

func NewSFUConfig() (*SFUConfig, error) {
	me := webrtc.MediaEngine{}
	err := me.RegisterDefaultCodecs()
	if err != nil {
		return nil, err
	}
	se := webrtc.SettingEngine{}
	udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IP{0, 0, 0, 0},
		Port: 50000,
	})
	if err != nil {
		panic(err)
	}
	se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))
	se.SetLite(true)
	return &SFUConfig{
		me: me,
		se: se,
	}, nil
}
