package rtc

import "github.com/pion/webrtc/v3"

type RTPParameters struct {
	Mid              string
	Encodings        []webrtc.RTPEncodingParameters
	HeaderExtensions []webrtc.RTPHeaderExtensionParameter
	Codecs           []webrtc.RTPCodecParameters
}

type RTCPFeedback struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter"`
}
type RTPCodec struct {
	// webrtc.RTPCodecCapability
	MimeType     string         `json:"mimeType"`
	ClockRate    uint32         `json:"clockRate"`
	Channels     uint16         `json:"channels,omitempty"`
	SDPFmtpLine  string         `json:"fmtp"`
	RTCPFeedback []RTCPFeedback `json:"rtcpFeedback"`

	// extended
	Kind                 string                 `json:"kind"`
	Parameters           map[string]interface{} `json:"parameters"`
	PreferredPayloadType int                    `json:"preferredPayloadType"`
}

type RTPHeaderExtension struct {
	// webrtc.RTPHeaderExtensionCapability
	URI string `json:"uri"`

	// extended
	Kind             string `json:"kind"`
	PreferredID      int    `json:"preferredId"`
	PreferredEncrypt bool   `json:"preferredEncrypt"`
	Direction        string `json:"direction"`
}

type RTPCapabilities struct {
	Codecs           []RTPCodec           `json:"codecs"`
	HeaderExtensions []RTPHeaderExtension `json:"headerExtensions"`
}
