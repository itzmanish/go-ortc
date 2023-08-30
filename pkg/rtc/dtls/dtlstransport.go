package dtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/itzmanish/go-ortc/v2/internal/mux"
	"github.com/itzmanish/go-ortc/v2/internal/util"
	"github.com/itzmanish/go-ortc/v2/pkg/buffer"
	"github.com/itzmanish/go-ortc/v2/pkg/logger"
	"github.com/itzmanish/go-ortc/v2/pkg/rtc/ice"
	"github.com/pion/dtls/v2"
	"github.com/pion/dtls/v2/pkg/crypto/fingerprint"
	"github.com/pion/rtcp"
	"github.com/pion/srtp/v2"
	"github.com/pion/webrtc/v3/pkg/rtcerr"
)

// DTLSTransport allows an application access to information about the DTLS
// transport over which RTP and RTCP packets are sent and received by
// RTPSender and RTPReceiver, as well other data such as SCTP packets sent
// and received by data channels.
type DTLSTransport struct {
	lock sync.RWMutex

	certificates          []Certificate
	remoteParameters      DTLSParameters
	remoteCertificate     []byte
	state                 DTLSTransportState
	srtpProtectionProfile srtp.ProtectionProfile

	onStateChangeHandler func(DTLSTransportState)

	conn         *dtls.Conn
	iceTransport *ice.ICETransport

	srtpSession, srtcpSession   atomic.Value
	srtpEndpoint, srtcpEndpoint *mux.Endpoint
	simulcastStreams            []*srtp.ReadStreamSRTP
	srtpReady                   chan struct{}

	bufferFactory *buffer.Factory
	log           logger.Logger
}

// NewDTLSTransport creates a new DTLSTransport.
// This constructor is part of the ORTC API. It is not
// meant to be used together with the basic WebRTC API.
func NewDTLSTransport(iceServer *ice.ICETransport, bufferFactory *buffer.Factory) (*DTLSTransport, error) {
	t := &DTLSTransport{
		state:         DTLSTransportStateNew,
		srtpReady:     make(chan struct{}),
		bufferFactory: bufferFactory,
		iceTransport:  iceServer,
		log:           logger.NewLogger("DTLSTransport"),
	}

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, &rtcerr.UnknownError{Err: err}
	}
	certificate, err := GenerateCertificate(sk)
	if err != nil {
		return nil, err
	}
	t.certificates = []Certificate{*certificate}

	return t, nil
}

// ICETransport returns the currently-configured *ICETransport or nil
// if one has not been configured
// func (t *DTLSTransport) ICETransport() *ICETransport {
// 	t.lock.RLock()
// 	defer t.lock.RUnlock()
// 	return t.iceTransport
// }

// onStateChange requires the caller holds the lock
func (t *DTLSTransport) onStateChange(state DTLSTransportState) {
	t.state = state
	handler := t.onStateChangeHandler
	if handler != nil {
		handler(state)
	}
}

// OnStateChange sets a handler that is fired when the DTLS
// connection state changes.
func (t *DTLSTransport) OnStateChange(f func(DTLSTransportState)) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.onStateChangeHandler = f
}

// State returns the current dtls transport state.
func (t *DTLSTransport) State() DTLSTransportState {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.state
}

// WriteRTCP sends a user provided RTCP packet to the connected peer. If no peer is connected the
// packet is discarded.
func (t *DTLSTransport) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	raw, err := rtcp.Marshal(pkts)
	if err != nil {
		return 0, err
	}

	srtcpSession, err := t.getSRTCPSession()
	if err != nil {
		return 0, err
	}

	writeStream, err := srtcpSession.OpenWriteStream()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errPeerConnWriteRTCPOpenWriteStream, err)
	}

	n, err := writeStream.Write(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// GetLocalParameters returns the DTLS parameters of the local DTLSTransport upon construction.
func (t *DTLSTransport) GetLocalParameters() (DTLSParameters, error) {
	fingerprints := []DTLSFingerprint{}

	for _, c := range t.certificates {
		prints, err := c.GetFingerprints()
		if err != nil {
			return DTLSParameters{}, err
		}

		fingerprints = append(fingerprints, prints...)
	}

	return DTLSParameters{
		Role:         DTLSRoleAuto, // always returns the default role
		Fingerprints: fingerprints,
	}, nil
}

// GetRemoteCertificate returns the certificate chain in use by the remote side
// returns an empty list prior to selection of the remote certificate
func (t *DTLSTransport) GetRemoteCertificate() []byte {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.remoteCertificate
}

func (t *DTLSTransport) startSRTP() error {
	srtpConfig := &srtp.Config{
		Profile:       t.srtpProtectionProfile,
		BufferFactory: t.bufferFactory.GetOrNew,
	}

	connState := t.conn.ConnectionState()
	err := srtpConfig.ExtractSessionKeysFromDTLS(&connState, t.role() == DTLSRoleClient)
	if err != nil {
		return fmt.Errorf("%w: %v", errDtlsKeyExtractionFailed, err)
	}

	srtpSession, err := srtp.NewSessionSRTP(t.srtpEndpoint, srtpConfig)
	if err != nil {
		return fmt.Errorf("%w: %v", errFailedToStartSRTP, err)
	}

	srtcpSession, err := srtp.NewSessionSRTCP(t.srtcpEndpoint, srtpConfig)
	if err != nil {
		return fmt.Errorf("%w: %v", errFailedToStartSRTCP, err)
	}

	t.srtpSession.Store(srtpSession)
	t.srtcpSession.Store(srtcpSession)
	close(t.srtpReady)
	return nil
}

func (t *DTLSTransport) getSRTPSession() (*srtp.SessionSRTP, error) {
	if value, ok := t.srtpSession.Load().(*srtp.SessionSRTP); ok {
		return value, nil
	}

	return nil, errDtlsTransportNotStarted
}

func (t *DTLSTransport) getSRTCPSession() (*srtp.SessionSRTCP, error) {
	if value, ok := t.srtcpSession.Load().(*srtp.SessionSRTCP); ok {
		return value, nil
	}

	return nil, errDtlsTransportNotStarted
}

func (t *DTLSTransport) role() DTLSRole {
	// If remote has an explicit role use the inverse
	switch t.remoteParameters.Role {
	case DTLSRoleClient:
		return DTLSRoleServer
	case DTLSRoleServer:
		return DTLSRoleClient
	default:
	}

	return defaultDtlsRoleAnswer
}

// Start DTLS transport negotiation with the parameters of the remote DTLS transport
func (t *DTLSTransport) Start(remoteParameters DTLSParameters) error {
	// Take lock and prepare connection, we must not hold the lock
	// when connecting
	prepareTransport := func() (DTLSRole, *dtls.Config, error) {
		t.lock.Lock()
		defer t.lock.Unlock()

		if err := t.ensureICEConn(); err != nil {
			return DTLSRole(0), nil, err
		}

		if t.state != DTLSTransportStateNew {
			return DTLSRole(0), nil, &rtcerr.InvalidStateError{Err: fmt.Errorf("%w: %s", errInvalidDTLSStart, t.state)}
		}

		t.srtpEndpoint = t.iceTransport.NewEndpoint(mux.MatchSRTP)
		t.srtcpEndpoint = t.iceTransport.NewEndpoint(mux.MatchSRTCP)
		t.remoteParameters = remoteParameters

		cert := t.certificates[0]
		t.onStateChange(DTLSTransportStateConnecting)

		return t.role(), &dtls.Config{
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{cert.x509Cert.Raw},
					PrivateKey:  cert.privateKey,
				},
			},
			SRTPProtectionProfiles: func() []dtls.SRTPProtectionProfile {
				return defaultSrtpProtectionProfiles()
			}(),
			ClientAuth:         dtls.RequireAnyClientCert,
			InsecureSkipVerify: true,
		}, nil
	}

	var dtlsConn *dtls.Conn
	dtlsEndpoint := t.iceTransport.NewEndpoint(mux.MatchDTLS)
	role, dtlsConfig, err := prepareTransport()
	if err != nil {
		return err
	}

	// Connect as DTLS Client/Server, function is blocking and we
	// must not hold the DTLSTransport lock
	if role == DTLSRoleClient {
		dtlsConn, err = dtls.Client(dtlsEndpoint, dtlsConfig)
	} else {
		dtlsConn, err = dtls.Server(dtlsEndpoint, dtlsConfig)
	}

	// Re-take the lock, nothing beyond here is blocking
	t.lock.Lock()
	defer t.lock.Unlock()

	if err != nil {
		t.onStateChange(DTLSTransportStateFailed)
		return err
	}

	srtpProfile, ok := dtlsConn.SelectedSRTPProtectionProfile()
	if !ok {
		t.onStateChange(DTLSTransportStateFailed)
		return ErrNoSRTPProtectionProfile
	}

	switch srtpProfile {
	case dtls.SRTP_AEAD_AES_128_GCM:
		t.srtpProtectionProfile = srtp.ProtectionProfileAeadAes128Gcm
	case dtls.SRTP_AES128_CM_HMAC_SHA1_80:
		t.srtpProtectionProfile = srtp.ProtectionProfileAes128CmHmacSha1_80
	default:
		t.onStateChange(DTLSTransportStateFailed)
		return ErrNoSRTPProtectionProfile
	}

	// Check the fingerprint if a certificate was exchanged
	remoteCerts := dtlsConn.ConnectionState().PeerCertificates
	if len(remoteCerts) == 0 {
		t.onStateChange(DTLSTransportStateFailed)
		return errNoRemoteCertificate
	}
	t.remoteCertificate = remoteCerts[0]

	t.conn = dtlsConn
	t.onStateChange(DTLSTransportStateConnected)

	return t.startSRTP()
}

// Stop stops and closes the DTLSTransport object.
func (t *DTLSTransport) Stop() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	// Try closing everything and collect the errors
	var closeErrs []error

	if srtpSession, err := t.getSRTPSession(); err == nil && srtpSession != nil {
		closeErrs = append(closeErrs, srtpSession.Close())
	}

	if srtcpSession, err := t.getSRTCPSession(); err == nil && srtcpSession != nil {
		closeErrs = append(closeErrs, srtcpSession.Close())
	}

	for i := range t.simulcastStreams {
		closeErrs = append(closeErrs, t.simulcastStreams[i].Close())
	}

	if t.conn != nil {
		// dtls connection may be closed on sctp close.
		if err := t.conn.Close(); err != nil && !errors.Is(err, dtls.ErrConnClosed) {
			closeErrs = append(closeErrs, err)
		}
	}
	t.onStateChange(DTLSTransportStateClosed)
	return util.FlattenErrs(closeErrs)
}

func (t *DTLSTransport) validateFingerPrint(remoteCert *x509.Certificate) error {
	for _, fp := range t.remoteParameters.Fingerprints {
		hashAlgo, err := fingerprint.HashFromString(fp.Algorithm)
		if err != nil {
			return err
		}

		remoteValue, err := fingerprint.Fingerprint(remoteCert, hashAlgo)
		if err != nil {
			return err
		}

		if strings.EqualFold(remoteValue, fp.Value) {
			return nil
		}
	}

	return errNoMatchingCertificateFingerprint
}

func (t *DTLSTransport) ensureICEConn() error {
	if t.iceTransport == nil {
		return errICEConnectionNotStarted
	}

	return nil
}

func (t *DTLSTransport) storeSimulcastStream(s *srtp.ReadStreamSRTP) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.simulcastStreams = append(t.simulcastStreams, s)
}
