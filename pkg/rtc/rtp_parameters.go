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

// =========== From ORTC to webrtc ==============

func ParseRTPCodecCapabilityFromORTC(caps RTPCodecCapability) webrtc.RTPCodecCapability {
	return webrtc.RTPCodecCapability{
		MimeType:     caps.MimeType,
		ClockRate:    caps.ClockRate,
		Channels:     caps.Channels,
		SDPFmtpLine:  caps.SDPFmtpLine,
		RTCPFeedback: ParseRTCPFeedbackFromORTC(caps.RTCPFeedback),
	}
}

func ParseRTPCodecParameterFromORTC(codec RTPCodecParameters) webrtc.RTPCodecParameters {
	return webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     codec.MimeType,
			ClockRate:    codec.ClockRate,
			Channels:     codec.Channels,
			SDPFmtpLine:  ParseParametersToFMTP(codec.Parameters),
			RTCPFeedback: ParseRTCPFeedbackFromORTC(codec.RTCPFeedback),
		},
		PayloadType: webrtc.PayloadType(codec.PayloadType),
	}
}

func ParseHeaderExtensionFromORTC(hext RTPHeaderExtension) webrtc.RTPHeaderExtensionCapability {
	return webrtc.RTPHeaderExtensionCapability{
		URI: hext.URI,
	}
}

func ParseHeaderExtensionParametersFromORTC(hext RTPHeaderExtensionParameter) webrtc.RTPHeaderExtensionParameter {
	return webrtc.RTPHeaderExtensionParameter{
		URI: hext.URI,
		ID:  hext.Id,
	}
}

func ParseRTPCapabilitiesFromORTC(caps RTPCapabilities) webrtc.RTPCapabilities {
	parsed := webrtc.RTPCapabilities{}
	for _, codec := range caps.Codecs {
		parsed.Codecs = append(parsed.Codecs, ParseRTPCodecCapabilityFromORTC(codec))
	}
	for _, hExt := range caps.HeaderExtensions {
		parsed.HeaderExtensions = append(parsed.HeaderExtensions, ParseHeaderExtensionFromORTC(hExt))
	}
	return parsed
}

func ParseRTPParametersFromORTC(params RTPParameters) webrtc.RTPParameters {
	parsed := webrtc.RTPParameters{}
	for _, codec := range params.Codecs {
		parsed.Codecs = append(parsed.Codecs, ParseRTPCodecParameterFromORTC(codec))
	}
	for _, hExt := range params.HeaderExtensions {
		parsed.HeaderExtensions = append(parsed.HeaderExtensions, ParseHeaderExtensionParametersFromORTC(hExt))
	}
	return parsed
}

func ParseRTCPFeedbackFromORTC(fb []RTCPFeedback) []webrtc.RTCPFeedback {
	feedback := []webrtc.RTCPFeedback{}
	for _, fbb := range fb {
		feedback = append(feedback, webrtc.RTCPFeedback{
			Type:      fbb.Type,
			Parameter: fbb.Parameter,
		})
	}
	return feedback
}

func ParseRTPReciveParametersFromORTC(parameters RTPReceiveParameters) webrtc.RTPReceiveParameters {
	receivingParameters := webrtc.RTPReceiveParameters{
		Encodings: make([]webrtc.RTPDecodingParameters, len(parameters.Encodings)),
	}
	for i, params := range parameters.Encodings {
		converted := webrtc.RTPDecodingParameters{
			RTPCodingParameters: webrtc.RTPCodingParameters{
				RID:         params.Rid,
				SSRC:        params.Ssrc,
				PayloadType: params.CodecPayloadType,
			},
		}
		if params.Rtx != nil {
			converted.RTX = *params.Rtx
		} else {
			converted.RTX = webrtc.RTPRtxParameters{}
		}
		receivingParameters.Encodings[i] = converted
	}
	return receivingParameters
}

// =============== To ORTC from webrtc

func ParseRTPSendParametersToORTC(params webrtc.RTPSendParameters) RTPSendParameters {
	parsed := RTPSendParameters{
		RTPParameters: ParseRTPParametersToORTC(params.RTPParameters),
	}
	for _, enc := range params.Encodings {
		parsed.Encodings = append(parsed.Encodings, ParseRTPEncodingToORTC(enc))
	}
	return parsed
}

func ParseRTPParametersToORTC(params webrtc.RTPParameters) RTPParameters {
	parsed := RTPParameters{}
	for _, codec := range params.Codecs {
		parsed.Codecs = append(parsed.Codecs, ParseRTPCodecParameterToORTC(codec))
	}
	for _, hExt := range params.HeaderExtensions {
		parsed.HeaderExtensions = append(parsed.HeaderExtensions, ParseHeaderExtensionParametersToORTC(hExt))
	}
	return parsed
}

func ParseRTPCodecParameterToORTC(codec webrtc.RTPCodecParameters) RTPCodecParameters {
	return RTPCodecParameters{
		MimeType:     codec.MimeType,
		ClockRate:    codec.ClockRate,
		Channels:     codec.Channels,
		Parameters:   ParseFMTPToParameters(codec.SDPFmtpLine),
		RTCPFeedback: ParseRTCPFeedbackToORTC(codec.RTCPFeedback),
		PayloadType:  uint8(codec.PayloadType),
	}
}

func ParseRTPCodecCapabilityToORTC(codec webrtc.RTPCodecCapability, kind MediaKind) RTPCodecCapability {
	parsedKind := "audio"
	if kind == AudioMediaKind {
		parsedKind = "audio"
	} else {
		parsedKind = "video"
	}
	return RTPCodecCapability{
		MimeType:     codec.MimeType,
		ClockRate:    codec.ClockRate,
		Channels:     codec.Channels,
		SDPFmtpLine:  codec.SDPFmtpLine,
		RTCPFeedback: ParseRTCPFeedbackToORTC(codec.RTCPFeedback),
		Kind:         parsedKind,
	}
}

func ParseRTPCapabilitiesToORTC(caps webrtc.RTPCapabilities, kind MediaKind) RTPCapabilities {
	parsed := RTPCapabilities{}
	for _, codec := range caps.Codecs {
		parsed.Codecs = append(parsed.Codecs, ParseRTPCodecCapabilityToORTC(codec, kind))
	}
	for _, header := range caps.HeaderExtensions {
		parsed.HeaderExtensions = append(parsed.HeaderExtensions, ParseHeaderExtensionToORTC(header, kind))
	}
	return parsed
}

func ParseHeaderExtensionToORTC(hext webrtc.RTPHeaderExtensionCapability, kind MediaKind) RTPHeaderExtension {
	parsedKind := "audio"
	if kind == AudioMediaKind {
		parsedKind = "audio"
	} else {
		parsedKind = "video"
	}
	return RTPHeaderExtension{
		URI:  hext.URI,
		Kind: parsedKind,
	}
}

func ParseHeaderExtensionParametersToORTC(hext webrtc.RTPHeaderExtensionParameter) RTPHeaderExtensionParameter {
	return RTPHeaderExtensionParameter{
		URI: hext.URI,
		Id:  hext.ID,
	}
}

func ParseRTCPFeedbackToORTC(fb []webrtc.RTCPFeedback) []RTCPFeedback {
	feedback := []RTCPFeedback{}
	for _, fbb := range fb {
		feedback = append(feedback, RTCPFeedback{
			Type:      fbb.Type,
			Parameter: fbb.Parameter,
		})
	}
	return feedback
}

func ParseRTPEncodingToORTC(encodings webrtc.RTPEncodingParameters) RTPEncodingParameters {
	return RTPEncodingParameters{
		RTPCodingParameters: RTPCodingParameters{
			Rid:              encodings.RID,
			Ssrc:             encodings.SSRC,
			Rtx:              &encodings.RTX,
			CodecPayloadType: encodings.PayloadType,
		},
	}
}

//  ======================

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
