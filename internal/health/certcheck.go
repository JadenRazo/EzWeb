package health

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// CheckCertExpiry connects to a domain via TLS and returns the leaf
// certificate's expiry time. A 10-second dial timeout prevents the health
// checker from stalling on unresponsive hosts. Returns a zero time and an
// error if the connection or certificate retrieval fails.
func CheckCertExpiry(domain string) (time.Time, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(
		dialer,
		"tcp",
		domain+":443",
		&tls.Config{InsecureSkipVerify: false},
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("TLS dial failed for %s: %w", domain, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, fmt.Errorf("no certificates returned for %s", domain)
	}

	// Index 0 is always the leaf (server) certificate.
	return certs[0].NotAfter, nil
}
