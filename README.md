# ORTC SFU written in go

This is very basic SFU writting with pion/webrtc without using Peerconnection
and SDP. This follows ORTC APIs.

## Current capabilities and goals

- [x] Media Forwarding (Audio/Video)
- [x] Incoming RTCP handling / Reciever Report generation
- [x] Sending RTCP Feedbacks / Sending report generation
- [x] TWCC
- [x] GCC for Sender Side BWE
- [x] REMB
- [x] Nack over same SSRC for Incoming RTP
- [x] Some additional header extension support(SDES:MID/ABS send time/time offset/video orientation)
- [x] Nack for outgoing RTP
- [ ] Datachannels over SCTP
- [ ] Simulcast (pion doesn't expose handler for unhandledSSRC)
- [ ] RTX support for incoming RTP (pion limitation)
