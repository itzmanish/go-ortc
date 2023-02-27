package rtc

import (
	"os"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/logging"
	"github.com/pion/sdp/v3"
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
	err = se.SetEphemeralUDPPortRange(40000, 50000)
	if err != nil {
		return nil, err
	}
	se.DisableMediaEngineCopy(true)
	se.SetLite(true)
	return &SFUConfig{
		me: me,
		se: se,
	}, nil
}

func DefaultHeaderExtensions(kind MediaKind) []RTPHeaderExtension {
	if kind == AudioMediaKind {
		return []RTPHeaderExtension{
			{URI: sdp.SDESMidURI, Kind: "audio"},
			{URI: sdp.SDESRTPStreamIDURI, Kind: "audio"},
			{URI: sdp.AudioLevelURI, Kind: "audio"},
		}
	}
	return []RTPHeaderExtension{
		{URI: sdp.SDESMidURI, Kind: "video"},
		{URI: sdp.SDESRTPStreamIDURI, Kind: "video"},
		{URI: sdp.TransportCCURI, Kind: "video"},
		{URI: "urn:ietf:params:rtp-hdrext:framemarking", Kind: "video"},
	}
}

func DefaultCodecs(kind MediaKind) []RTPCodec {
	if kind == AudioMediaKind {
		return []RTPCodec{
			{
				MimeType:    webrtc.MimeTypeOpus,
				ClockRate:   48000,
				Channels:    2,
				SDPFmtpLine: "minptime=10;useinbandfec=1",
				Kind:        "audio",
			},
		}
	}
	videoRTCPFeedback := []RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}}
	return []RTPCodec{
		{
			MimeType:     webrtc.MimeTypeVP8,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "",
			RTCPFeedback: videoRTCPFeedback,
			Kind:         "video",
		},
		{
			MimeType:    "video/rtx",
			Channels:    0,
			ClockRate:   90000,
			SDPFmtpLine: "apt=96",
			Kind:        "video",
		},
	}
}

func DefaultRouterCapabilities() RTPCapabilities {
	codecs := DefaultCodecs(AudioMediaKind)
	codecs = append(codecs, DefaultCodecs(VideoMediaKind)...)
	headerExts := DefaultHeaderExtensions(AudioMediaKind)
	headerExts = append(headerExts, DefaultHeaderExtensions(VideoMediaKind)...)
	return RTPCapabilities{
		Codecs:           codecs,
		HeaderExtensions: headerExts,
	}
}
