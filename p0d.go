package p0d

import (
	"crypto/tls"
	"fmt"
	"github.com/hako/durafmt"
	"github.com/rs/zerolog/log"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const Version string = "0.1.1"

type P0d struct {
	Config Config
	Client *http.Client
	Log    []ReqAtmpt
	Start  time.Time
	Stop   time.Time
}

type ReqAtmpt struct {
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
		Client: cfg.scaffoldHttpClient(),
		Log:    make([]ReqAtmpt, 0),
		Start:  time.Now(),
	}
}

func NewP0dFromFile(f string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg = cfg.validate()
	return &P0d{
		Config: *cfg,
		Client: cfg.scaffoldHttpClient(),
		Log:    make([]ReqAtmpt, 0),
		Start:  time.Now(),
	}
}

func (p *P0d) Race() {
	log.Info().Msgf("p0d %s starting with %d thread(s) using %d max TCP connection(s) hitting url %s for %d second(s)...",
		Version,
		p.Config.Exec.Threads,
		p.Config.Exec.Connections,
		p.Config.Req.Url,
		p.Config.Exec.DurationSeconds)

	end := make(chan struct{})
	ras := make(chan ReqAtmpt, 1000)

	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		end <- struct{}{}
	})

	for i := 0; i < p.Config.Exec.Threads; i++ {
		go p.doReqAtmpt(ras)
	}

	for {
		select {
		case <-end:
			p.Stop = time.Now()
			elapsed := durafmt.Parse(p.Stop.Sub(p.Start)).LimitFirstN(2).String()
			wrap := func(vs ...interface{}) []interface{} {
				return vs
			}
			log.Info().Msgf("p0d exiting after %d requests, runtime %s, avg %d req/s...", len(p.Log), elapsed, len(p.Log)/p.Config.Exec.DurationSeconds)
			log.Info().Msgf("p0d matching response codes (%d/%d) %s%%", wrap(p.Config.matchingResponseCodes(p.Log))...)
			log.Info().Msgf("p0d errors (%d/%d) %s%%", wrap(p.Config.errorCount(p.Log))...)

			os.Exit(0)
		case ra := <-ras:
			p.Log = append(p.Log, ra)
		}
	}

}

func (p *P0d) doReqAtmpt(ras chan<- ReqAtmpt) {
	for {
		req, _ := http.NewRequest(p.Config.Req.Method,
			p.Config.Req.Url,
			strings.NewReader(p.Config.Req.Body))
		ra := ReqAtmpt{
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
		res, e := p.Client.Do(req)
		if res != nil {
			ra.ResponseCode = res.StatusCode
			io.Copy(ioutil.Discard, res.Body)
			res.Body.Close()
		}

		ra.Stop = time.Now()
		ra.ResponseError = e
		ras <- ra
	}
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
		IdleConnTimeout:     3 * time.Second,
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
