package p0d

import (
	"crypto/tls"
	"github.com/rs/zerolog/log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const Version string = "0.1"

type P0d struct {
	t      int
	c      int
	d      int
	u      string
	client *http.Client
}

func NewP0d(t int, c int, d int, u string) *P0d {
	return &P0d{
		t:      t,
		c:      c,
		d:      d,
		u:      u,
		client: scaffoldHttpClient(c),
	}
}

func (p *P0d) Race() {
	log.Info().Msgf("p0d starting with %d thread(s) using max %d connection(s) for %d second(s)...", p.t, p.c, p.d)
	wg := sync.WaitGroup{}
	time.AfterFunc(time.Duration(p.d)*time.Second, func() {
		for i := 0; i < p.t; i++ {
			log.Debug().Msgf("ending thread %d", i)
			wg.Done()
		}
	})

	wg.Add(p.t)
	for i := 0; i < p.t; i++ {
		go func(i int) {
			log.Debug().Msgf("starting thread %d", i)
			for {
				r, e := p.client.Get(p.u)
				if e != nil {
					log.Error().Err(e)
				} else {
					_ = r
				}
			}
		}(i)
	}

	wg.Wait()
	log.Info().Msg("exiting...")
	os.Exit(0)
}

func scaffoldHttpClient(maxConns int) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
			DialContext: (&net.Dialer{
				//we are aborting after 3 seconds of dial connect to complete and treat the dial as degraded
				Timeout: 3 * time.Second,
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
