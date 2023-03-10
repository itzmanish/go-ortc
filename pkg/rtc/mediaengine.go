package rtc

import (
	"strings"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func SetupConsumerMediaEngineWithProducerParams(me *webrtc.MediaEngine, params webrtc.RTPParameters, kind webrtc.RTPCodecType) error {
	for _, codec := range params.Codecs {
		err := me.RegisterCodec(codec, kind)
		if err != nil {
			return err
		}
	}
	for _, hExt := range params.HeaderExtensions {
		err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{
			URI: hExt.URI,
		}, kind)
		if err != nil {
			return err
		}
	}
	return nil
}

func SetupProducerMediaEngineForKind(me *webrtc.MediaEngine, kind MediaKind) error {
	for _, codec := range GetCodecsForKind(kind) {
		err := me.RegisterCodec(codec, webrtc.RTPCodecType(kind))
		if err != nil {
			return err
		}
	}
	for _, hExt := range GetHeaderExtensionForKind(kind) {
		err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{
			URI: hExt.URI,
		}, webrtc.RTPCodecType(kind))
		if err != nil {
			return err
		}
	}

	return nil
}

func GetCodecsForKind(kind MediaKind) []webrtc.RTPCodecParameters {
	switch kind {
	case AudioMediaKind:
		return []webrtc.RTPCodecParameters{
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
				PayloadType:        111,
			},
		}
	case VideoMediaKind:
		videoRTCPFeedback := []webrtc.RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}}
		return []webrtc.RTPCodecParameters{
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeVP8, 90000, 0, "", videoRTCPFeedback},
				PayloadType:        96,
			},
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{"video/rtx", 90000, 0, "apt=96", nil},
				PayloadType:        97,
			},
		}
	}
	return []webrtc.RTPCodecParameters{}
}

func GetHeaderExtensionForKind(kind MediaKind) []webrtc.RTPHeaderExtensionParameter {
	audio := []webrtc.RTPHeaderExtensionParameter{
		webrtc.RTPHeaderExtensionParameter{
			URI: sdp.SDESMidURI,
			ID:  1,
		},
		webrtc.RTPHeaderExtensionParameter{
			URI: sdp.SDESRTPStreamIDURI,
			ID:  2,
		},
		webrtc.RTPHeaderExtensionParameter{
			URI: sdp.AudioLevelURI,
			ID:  10,
		},
	}
	video := []webrtc.RTPHeaderExtensionParameter{
		webrtc.RTPHeaderExtensionParameter{
			URI: sdp.SDESMidURI,
			ID:  1,
		},
		webrtc.RTPHeaderExtensionParameter{
			URI: sdp.SDESRTPStreamIDURI,
			ID:  2,
		},
		webrtc.RTPHeaderExtensionParameter{
			URI: sdp.TransportCCURI,
			ID:  5,
		},
		webrtc.RTPHeaderExtensionParameter{
			URI: "urn:ietf:params:rtp-hdrext:framemarking",
			ID:  7,
		},
	}
	if kind == AudioMediaKind {
		return audio
	}
	if kind == VideoMediaKind {
		return video
	}

	return []webrtc.RTPHeaderExtensionParameter{}
}

func DefaultRouterCapabilities() RTPCapabilities {
	caps := RTPCapabilities{}
	acodecs := GetCodecsForKind(AudioMediaKind)
	vcodecs := GetCodecsForKind(VideoMediaKind)
	parsedHExts := []RTPHeaderExtension{}

	for _, hExt := range GetHeaderExtensionForKind(AudioMediaKind) {
		parsedHExts = append(parsedHExts, RTPHeaderExtension{
			URI:         hExt.URI,
			Kind:        "audio",
			PreferredID: hExt.ID,
		})
	}
	for _, hExt := range GetHeaderExtensionForKind(VideoMediaKind) {
		parsedHExts = append(parsedHExts, RTPHeaderExtension{
			URI:         hExt.URI,
			Kind:        "video",
			PreferredID: hExt.ID,
		})
	}
	for _, codec := range acodecs {
		caps.Codecs = append(caps.Codecs, RTPCodecCapability{
			Name:                 codec.MimeType,
			MimeType:             codec.MimeType,
			PreferredPayloadType: uint8(codec.PayloadType),
			ClockRate:            codec.ClockRate,
			Channels:             codec.Channels,
			Parameters:           ParseFMTPToParameters(codec.SDPFmtpLine),
			RTCPFeedback:         ParseRTCPFeedbackToORTC(codec.RTCPFeedback),
			Kind:                 "audio",
		})
	}

	for _, codec := range vcodecs {
		caps.Codecs = append(caps.Codecs, RTPCodecCapability{
			Name:                 codec.MimeType,
			MimeType:             codec.MimeType,
			PreferredPayloadType: uint8(codec.PayloadType),
			ClockRate:            codec.ClockRate,
			Channels:             codec.Channels,
			Parameters:           ParseFMTPToParameters(codec.SDPFmtpLine),
			RTCPFeedback:         ParseRTCPFeedbackToORTC(codec.RTCPFeedback),
			Kind:                 "video",
		})
	}
	caps.HeaderExtensions = parsedHExts
	return caps
}

func RemoveRTXCodecsFromRTPParameters(params webrtc.RTPParameters) webrtc.RTPParameters {
	converted := webrtc.RTPParameters{
		HeaderExtensions: params.HeaderExtensions,
	}
	for _, codec := range params.Codecs {
		if !strings.Contains(codec.MimeType, "rtx") {
			converted.Codecs = append(converted.Codecs, codec)
		}
	}
	return converted
}
