package p0d

import (
	"crypto/tls"
	"fmt"
	"github.com/google/uuid"
	"github.com/hako/durafmt"
	"github.com/k0kubun/go-ansi"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const Version string = "v0.1.3"

type P0d struct {
	ID     string
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
		ID:     createRunId(),
		Config: cfg,
		Client: cfg.scaffoldHttpClient(),
		Log:    make([]ReqAtmpt, 0),
		Start:  time.Now(),
	}
}

func createRunId() string {
	uid, _ := uuid.NewRandom()
	return fmt.Sprintf("p0d-%s-%s", Version, uid)
}

func NewP0dFromFile(f string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg = cfg.validate()

	return &P0d{
		ID:     createRunId(),
		Config: *cfg,
		Client: cfg.scaffoldHttpClient(),
		Log:    make([]ReqAtmpt, 0),
		Start:  time.Now(),
	}
}

func (p *P0d) Race() {
	log.Info().Msgf("%s starting...", p.ID)
	log.Info().Msgf("duration: %s", durafmt.Parse(time.Duration(p.Config.Exec.DurationSeconds)*time.Second).LimitFirstN(2).String())
	log.Info().Msgf("thread(s): %d", p.Config.Exec.Threads)
	log.Info().Msgf("max conn(s): %d", p.Config.Exec.Connections)
	log.Info().Msgf("%s %s", p.Config.Req.Method, p.Config.Req.Url)

	end := make(chan struct{})
	ras := make(chan ReqAtmpt, 10000)

	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		end <- struct{}{}
	})

	for i := 0; i < p.Config.Exec.Threads; i++ {
		go p.doReqAtmpt(ras)
	}

	bar := p.initProgressBar()
	for {
		select {
		case <-end:
			//just in case the progress bar didn't update to 100% cleanly with the exit signal
			bar.Set(p.Config.Exec.DurationSeconds)

			p.Stop = time.Now()
			elapsed := durafmt.Parse(p.Stop.Sub(p.Start)).LimitFirstN(2).String()
			wrap := func(vs ...interface{}) []interface{} {
				return vs
			}

			//fix issue with progress bar, force newline
			os.Stdout.Write([]byte("\n"))
			log.Info().Msg("")
			log.Info().Msg("|--------------|")
			log.Info().Msg("| Test summary |")
			log.Info().Msg("|--------------|")
			log.Info().Msgf("ID: %s", p.ID)
			log.Info().Msgf("runtime: %s", elapsed)
			log.Info().Msgf("total requests: %s", FGroup(int64(len(p.Log))))
			log.Info().Msgf("avg HTTP req/s: %s", FGroup(int64(len(p.Log)/p.Config.Exec.DurationSeconds)))
			log.Info().Msgf("matching HTTP response codes: %s/%s (%s%%)",
				wrap(p.Config.matchingResponseCodes(p.Log))...)
			log.Info().Msgf("transport errors: %s/%s (%s%%)", wrap(p.Config.errorCount(p.Log))...)

			os.Exit(0)
		case ra := <-ras:
			elpsd := time.Now().Sub(p.Start).Seconds()
			bar.Set(int(elpsd))
			p.Log = append(p.Log, ra)
		}
	}

}

func (p *P0d) initProgressBar() *progressbar.ProgressBar {
	start := time.Now().Format(time.Kitchen)
	return progressbar.NewOptions(p.Config.Exec.DurationSeconds,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(75),
		progressbar.OptionSetDescription(fmt.Sprintf("[dark_gray]%s[reset] sending HTTP requests...", start)),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[yellow]=[reset]",
			SaucerHead:    "[cyan]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))
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

func (cfg Config) matchingResponseCodes(log []ReqAtmpt) (string, string, string) {
	var match float32 = 0
	for _, c := range log {
		if c.ResponseCode == cfg.Res.Code {
			match++
		}
	}
	return FGroup(int64(match)), FGroup(int64(len(log))), fmt.Sprintf("%.2f", 100*(match/float32(len(log))))
}

func (cfg Config) errorCount(log []ReqAtmpt) (string, string, string) {
	var match float32 = 0
	for _, c := range log {
		if c.ResponseError != nil {
			match++
		}
	}
	return FGroup(int64(match)), FGroup(int64(len(log))), fmt.Sprintf("%.2f", 100*(match/float32(len(log))))
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

		MaxConnsPerHost:     cfg.Exec.Connections,
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

func FGroup(n int64) string {
	in := strconv.FormatInt(n, 10)
	numOfDigits := len(in)
	if n < 0 {
		numOfDigits-- // First character is the - sign (not a digit)
	}
	numOfCommas := (numOfDigits - 1) / 3

	out := make([]byte, len(in)+numOfCommas)
	if n < 0 {
		in, out[0] = in[1:], '-'
	}

	for i, j, k := len(in)-1, len(out)-1, 0; ; i, j = i-1, j-1 {
		out[j] = in[i]
		if i == 0 {
			return string(out)
		}
		if k++; k == 3 {
			j, k = j-1, 0
			out[j] = ','
		}
	}
}
