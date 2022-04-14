package p0d

import (
	"crypto/tls"
	"github.com/rs/zerolog/log"
	"net"
	"net/http"
	"os"
)

const Version string = "0.1"

type P0d struct {
	t      int
	c      int
	client *http.Client
}

func NewP0d(t int, c int) *P0d {
	return &P0d{
		t:      t,
		c:      c,
		client: scaffoldHttpClient(c),
	}
}

func (p *P0d) Race() {
	log.Info().Msgf("p0d starting with %d thread(s) using max %d connection(s)...", p.t, p.c)
	os.Exit(0)
}

func scaffoldHttpClient(maxConns int) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
			DialContext: (&net.Dialer{
				//we are aborting after 3 seconds of dial connect to complete and treat the dial as degraded
				Timeout: 3,
			}).DialContext,
			//TLS handshake timeout is the same as connection timeout
			TLSHandshakeTimeout: 3,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			MaxIdleConns:        maxConns,
			MaxIdleConnsPerHost: maxConns,
			IdleConnTimeout:     1,
		},
	}
}
