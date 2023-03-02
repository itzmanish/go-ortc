package rtc

import (
	"os"

	"github.com/itzmanish/go-ortc/pkg/buffer"
	"github.com/pion/logging"
	"github.com/pion/sdp/v3"
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
	err = se.SetEphemeralUDPPortRange(40000, 50000)
	if err != nil {
		return nil, err
	}
	se.DisableMediaEngineCopy(true)
	se.SetLite(true)
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
	videoRTCPFeedback := []RTCPFeedback{{"goog-remb", ""}, {"transport-cc", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}}
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

func GetMediaEngine(publisher bool) (*webrtc.MediaEngine, error) {
	m := &webrtc.MediaEngine{}
	if publisher {
		err := RegisterCodecs(m)
		if err != nil {
			return nil, err
		}
		err = RegisterHeaderExtension(m)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

func RegisterCodecs(me *webrtc.MediaEngine) error {
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
			PayloadType:        111,
		},
	} {
		if err := me.RegisterCodec(codec, webrtc.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}}
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeVP8, 90000, 0, "", videoRTCPFeedback},
			PayloadType:        96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{"video/rtx", 90000, 0, "apt=96", nil},
			PayloadType:        97,
		},
	} {
		if err := me.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}
	return nil
}

func RegisterHeaderExtension(me *webrtc.MediaEngine) error {

	audio := []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		sdp.AudioLevelURI,
	}
	video := []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		sdp.TransportCCURI,
		"urn:ietf:params:rtp-hdrext:framemarking",
	}
	for _, uri := range audio {
		err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: uri}, webrtc.RTPCodecTypeAudio)
		if err != nil {
			return err
		}
	}
	for _, uri := range video {
		err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: uri}, webrtc.RTPCodecTypeVideo)
		if err != nil {
			return err
		}
	}
	return nil
}
