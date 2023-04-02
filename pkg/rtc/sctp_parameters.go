package rtc

type SCTPStreamParameters struct {
	MaxPacketLifeTime uint `json:"maxPacketLifeTime,omitempty"`
	MaxRetransmits    uint `json:"maxRetransmits,omitempty"`
	Ordered           bool `json:"ordered,omitempty"`
	StreamID          uint `json:"streamId"`
}
