package p0d

import (
	"crypto/tls"
	"fmt"
	durafmt "github.com/hako/durafmt"
	"github.com/rs/zerolog/log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const Version string = "0.1"

type P0d struct {
	Config Config
	client *http.Client
	log    []ReqAtmpt
}

type ReqAtmpt struct {
	ct            int
	Start         time.Time
	Stop          time.Time
	Req           *http.Request
	ResponseCode  int
	ResponseBytes int
	ResponseError error
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
			Connections:     c,
		},
	}
	cfg = *cfg.validate()

	return &P0d{
		Config: cfg,
		client: cfg.scaffoldHttpClient(),
		log:    make([]ReqAtmpt, 0),
	}
}

func NewP0dFromFile(f string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg = cfg.validate()
	return &P0d{
		Config: *cfg,
		client: cfg.scaffoldHttpClient(),
		log:    make([]ReqAtmpt, 0),
	}
}

func (p *P0d) Race() {
	start := time.Now()

	log.Info().Msgf("p0d starting with %d thread(s) using %d max TCP connection(s) hitting url %s for %d second(s)...",
		p.Config.Exec.Threads,
		p.Config.Exec.Connections,
		p.Config.Req.Url,
		p.Config.Exec.DurationSeconds)

	wg := sync.WaitGroup{}
	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		for i := 0; i < p.Config.Exec.Threads; i++ {
			wg.Done()
		}
	})

	wg.Add(p.Config.Exec.Threads)
	for i := 0; i < p.Config.Exec.Threads; i++ {
		go func(i int) {
			var ct int = 0

			for {
				req, _ := http.NewRequest(p.Config.Req.Method,
					p.Config.Req.Url,
					strings.NewReader(p.Config.Req.Body))

				ra := ReqAtmpt{
					ct:    ct,
					Req:   req,
					Start: time.Now(),
				}

				if len(p.Config.Req.Headers) > 0 {
					for _, h := range p.Config.Req.Headers {
						for k, v := range h {
							req.Header.Add(k, v)
						}
					}
				}

				res, e := p.client.Do(req)
				ra.Stop = time.Now()
				if res != nil {
					ra.ResponseCode = res.StatusCode
				}
				ra.ResponseError = e

				p.log = append(p.log, ra)

				ct++
			}
		}(i)
	}

	wg.Wait()
	stop := time.Now()
	elapsed := durafmt.Parse(stop.Sub(start)).LimitFirstN(2).String()
	wrap := func(vs ...interface{}) []interface{} {
		return vs
	}
	log.Info().Msgf("p0d exiting after %d requests, runtime %s...", len(p.log), elapsed)
	log.Info().Msgf("matching response codes (%d/%d) %s%%", wrap(p.Config.matchingResponseCodes(p.log))...)
	log.Info().Msgf("errors (%d/%d) %s%%", wrap(p.Config.errorCount(p.log))...)

	os.Exit(0)
}

func (cfg Config) matchingResponseCodes(log []ReqAtmpt) (int, int, string) {
	var match float32 = 0
	for _, c := range log {
		if c.ResponseCode == cfg.Res.Code {
			match++
		}
	}
	return int(match), len(log), fmt.Sprintf("%.2f", 100*(match/float32(len(log))))
}

func (cfg Config) errorCount(log []ReqAtmpt) (int, int, string) {
	var match float32 = 0
	for _, c := range log {
		if c.ResponseError != nil {
			match++
		}
	}
	return int(match), len(log), fmt.Sprintf("%.2f", 100*(match/float32(len(log))))
}

func (cfg Config) scaffoldHttpClient() *http.Client {
	t := &http.Transport{
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
	}

	//see https://stackoverflow.com/questions/57683132/turning-off-connection-pool-for-go-http-client
	if cfg.Exec.Connections == UNLIMITED {
		log.Debug().Msg("transport connection pool disabled")
		t.DisableKeepAlives = true
	}

	return &http.Client{
		Transport: t,
	}
}
