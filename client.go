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
	Config Config
	client *http.Client
}

func NewP0dWithValues(t int, c int, d int, u string) *P0d {
	cfg := Config{
		Req: Req{
			Method: "GET",
			Url:    u,
		},
		Exec: Exec{
			Threads:         t,
			DurationSeconds: d,
		},
	}

	return &P0d{
		Config: cfg,
		client: cfg.scaffoldHttpClient(),
	}
}

func NewP0dFromFile(f string) *P0d {
	cfg := *loadConfigFromFile(f)
	return &P0d{
		Config: *loadConfigFromFile(f),
		client: cfg.scaffoldHttpClient(),
	}
}

func (p *P0d) Race() {
	log.Debug().Msgf("p0d starting with %d thread(s) using %d max TCP connection(s) hitting url %s for %d second(s)...",
		p.Config.Exec.Threads,
		p.Config.Exec.Connections,
		p.Config.Req.Url,
		p.Config.Exec.DurationSeconds)

	wg := sync.WaitGroup{}
	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		for i := 0; i < p.Config.Exec.Threads; i++ {
			log.Debug().Msgf("ending thread %d", i)
			wg.Done()
		}
	})

	wg.Add(p.Config.Exec.Threads)
	for i := 0; i < p.Config.Exec.Threads; i++ {
		go func(i int) {
			log.Debug().Msgf("starting thread %d", i)
			for {
				r, e := p.client.Get(p.Config.Req.Url)
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

func (cfg Config) scaffoldHttpClient() *http.Client {
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
			MaxIdleConns:        cfg.Exec.Connections,
			MaxIdleConnsPerHost: cfg.Exec.Connections,
			IdleConnTimeout:     1,
		},
	}
}
