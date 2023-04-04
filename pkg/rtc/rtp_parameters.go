package rtc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/pion/webrtc/v3"
)

type SSRC uint32
type PayloadType uint8

type RTPReceiveParameters struct {
	RTPParameters
	Encodings []RTPDecodingParameters `json:"encodings"`
}

type RTPSendParameters struct {
	RTPParameters
	Encodings []RTPEncodingParameters `json:"encodings"`
}
type RTPParameters struct {
	Mid              string                        `json:"mid"`
	HeaderExtensions []RTPHeaderExtensionParameter `json:"headerExtensions"`
	Codecs           []RTPCodecParameters          `json:"codecs"`
	Rtcp             RTCPParameters                `json:"rtcp"`
}

type RTCPParameters struct {
	SSRC        uint   `json:"ssrc,omitempty"`
	Cname       string `json:"cname"`
	ReducedSize bool   `json:"reducedSize"`
	Mux         bool   `json:"mux"`
}

type RTPRtxParameters struct {
	SSRC SSRC `json:"ssrc"`
}

type RTPCodecParameters struct {
	// https://draft.ortc.org/#dom-rtcrtpcodecparameters
	MimeType     string                 `json:"mimeType"`
	PayloadType  uint8                  `json:"payloadType"`
	ClockRate    uint32                 `json:"clockRate"`
	Channels     uint16                 `json:"channels,omitempty"`
	Maxptime     uint32                 `json:"maxptime,omitempty"`
	Ptime        uint32                 `json:"ptime,omitempty"`
	RTCPFeedback []RTCPFeedback         `json:"rtcpFeedback"`
	Parameters   map[string]interface{} `json:"parameters"`
}

type RTCPFeedback struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter"`
}

type RTPHeaderExtensionParameter struct {
	URI        string         `json:"uri"`
	Id         int            `json:"id"`
	Encrypt    bool           `json:"encrypt"`
	Parameters map[string]any `parameters`
}

type RTPEncodingParameters struct {
	RTPCodingParameters
	// RTCDtxStatus    dtx;
	// RTCPriorityType priority = "low";
	// unsigned long   maxBitrate;
	// double          resolutionScale;
	// double          framerateScale;
	// double          maxFramerate;
	Dtx string `json:"dtx,omitempty"` // "enable" || "disable"
	// I will implement them later
}

type RTPDecodingParameters struct {
	RTPCodingParameters
}
type RTPCodingParameters struct {
	Ssrc                  webrtc.SSRC        `json:"ssrc"`
	CodecPayloadType      webrtc.PayloadType `json:"codecPayloadType,omitempty"`
	Fec                   *RTPFecParameters  `json:"fec,omitempty"`
	Rtx                   RTPRtxParameters   `json:"rtx,omitempty"`
	Active                bool               `json:"active,omitempty"`
	Rid                   string             `json:"rid,omitempty"`
	EncodingId            string             `json:"encodingId,omitempty"`
	DependencyEncodingIds []string           `json:"dependencyEncodingIds,omitempty"`
}

type RTPFecParameters struct {
	Ssrc      SSRC   `json:"ssrc,omitempty"`
	Mechanism string `json:"mechanism,omitempty"`
}
type RTPCapabilities struct {
	Codecs           []RTPCodecCapability `json:"codecs"`
	HeaderExtensions []RTPHeaderExtension `json:"headerExtensions"`
}

type RTPCodecCapability struct {
	MimeType             string         `json:"mimeType"`
	PreferredPayloadType uint8          `json:"preferredPayloadType"`
	ClockRate            uint32         `json:"clockRate"`
	Channels             uint16         `json:"channels,omitempty"`
	Maxptime             uint32         `json:"maxptime,omitempty"`
	Ptime                uint32         `json:"ptime,omitempty"`
	SDPFmtpLine          string         `json:"fmtp,omitempty"`
	RTCPFeedback         []RTCPFeedback `json:"rtcpFeedback"`

	// extended
	Kind       string                 `json:"kind"`
	Parameters map[string]interface{} `json:"parameters"`
}

type RTPHeaderExtension struct {
	URI              string `json:"uri"`
	Kind             string `json:"kind"`
	PreferredID      int    `json:"preferredId"`
	PreferredEncrypt bool   `json:"preferredEncrypt"`
}

type MediaKind uint8

const (
	AudioMediaKind = iota + 1
	VideoMediaKind
)

func (mk MediaKind) String() string {
	if mk == AudioMediaKind {
		return "audio"
	}
	if mk == VideoMediaKind {
		return "video"
	}
	return "unknown"
}

func ParseParametersToFMTP(params map[string]any) string {
	parsed := ""
	for k, v := range params {
		parsed += fmt.Sprintf("%v=%v;", k, v)
	}
	parsed = strings.TrimSuffix(parsed, ";")
	return parsed
}

func ParseFMTPToParameters(fmtp string) map[string]any {
	parsed := map[string]any{}
	if len(fmtp) == 0 {
		return parsed
	}
	fmtps := strings.Split(strings.TrimSpace(fmtp), ";")
	for _, v := range fmtps {
		if len(v) == 0 {
			continue
		}
		ff := strings.Split(v, "=")
		iValue, err := strconv.Atoi(ff[1])
		if err != nil {
			logger.Error("failed to parse fmtp value to integer", ff[1])
		}
		parsed[ff[0]] = iValue
	}
	return parsed
}

func ConvertRTPRecieveParametersToRTPParamters(params RTPReceiveParameters) RTPParameters {
	return RTPParameters{
		Mid:              params.Mid,
		HeaderExtensions: params.HeaderExtensions,
		Codecs:           params.Codecs,
		Rtcp:             params.Rtcp,
	}
}

func ConvertRTPSendParametersToRTPReceiveParameters(params RTPSendParameters) RTPReceiveParameters {
	converted := RTPReceiveParameters{
		RTPParameters: params.RTPParameters,
	}
	for _, enc := range params.Encodings {
		converted.Encodings = append(converted.Encodings, RTPDecodingParameters{
			RTPCodingParameters: enc.RTPCodingParameters,
		})
	}
	return converted
}
