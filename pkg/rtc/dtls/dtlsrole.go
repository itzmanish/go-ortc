package dtls

// DTLSRole indicates the role of the DTLS transport.
type DTLSRole byte

const (
	// DTLSRoleAuto defines the DTLS role is determined based on
	// the resolved ICE role: the ICE controlled role acts as the DTLS
	// client and the ICE controlling role acts as the DTLS server.
	DTLSRoleAuto DTLSRole = iota + 1

	// DTLSRoleClient defines the DTLS client role.
	DTLSRoleClient

	// DTLSRoleServer defines the DTLS server role.
	DTLSRoleServer
)

const (
	// https://tools.ietf.org/html/rfc5763
	/*
		The answerer MUST use either a
		setup attribute value of setup:active or setup:passive.  Note that
		if the answerer uses setup:passive, then the DTLS handshake will
		not begin until the answerer is received, which adds additional
		latency. setup:active allows the answer and the DTLS handshake to
		occur in parallel.  Thus, setup:active is RECOMMENDED.
	*/
	defaultDtlsRoleAnswer = DTLSRoleClient
	/*
		The endpoint that is the offerer MUST use the setup attribute
		value of setup:actpass and be prepared to receive a client_hello
		before it receives the answer.
	*/
	defaultDtlsRoleOffer = DTLSRoleAuto
)

func (r DTLSRole) String() string {
	switch r {
	case DTLSRoleAuto:
		return "auto"
	case DTLSRoleClient:
		return "client"
	case DTLSRoleServer:
		return "server"
	default:
		return "unknown"
	}
}
