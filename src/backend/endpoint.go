package backend

import (
	"context"
	"crypto/tls"
	"net"
)

// Endpoint holds information about a back-end HTTP(s) server.
type Endpoint struct {
	// A human readable description of what the end-point is, not necessarily
	// unique to this endpoint.
	Description string

	// Address hosts the network address of the back-end server, including the
	// port number or name.
	Address string

	// IsTLS indicates whether or not the back-end server is expecting a TLS
	// connection. If true, the "https://" or "wss://" scheme is used; otherwise,
	// "http://" or "ws://" is used.
	IsTLS bool
}

// GetScheme produces the URL scheme that should be used to connect to the
// endpoint.
func (endpoint *Endpoint) GetScheme(isWebSocket bool) string {
	var scheme string
	if isWebSocket {
		scheme = "ws"
	} else {
		scheme = "http"
	}

	if endpoint.IsTLS {
		scheme += "s"
	}

	return scheme
}

// Dial connects to the endpoint using the appropriate dialer.
func (endpoint *Endpoint) Dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer

	if endpoint.IsTLS {
		if deadline, ok := ctx.Deadline(); ok {
			dialer.Deadline = deadline
		}

		return tls.DialWithDialer(&dialer, "tcp", endpoint.Address, nil)
	}

	return dialer.DialContext(ctx, "tcp", endpoint.Address)
}
