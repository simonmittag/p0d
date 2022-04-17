package p0d

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/hako/durafmt"
	"github.com/k0kubun/go-ansi"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

const Version string = "v0.1.9"

type P0d struct {
	ID     string
	Config Config
	Client *http.Client
	Stats  *Stats
	Start  time.Time
	Stop   time.Time
}

type ReqAtmpt struct {
	Start         time.Time
	Stop          time.Time
	Elapsed       time.Duration
	ResponseCode  int
	ResponseBytes int64
	ResponseError string
}

func createRunId() string {
	uid, _ := uuid.NewRandom()
	return fmt.Sprintf("p0d-%s-%s", Version, uid)
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

	start := time.Now()
	return &P0d{
		ID:     createRunId(),
		Config: cfg,
		Client: cfg.scaffoldHttpClient(),
		Stats: &Stats{
			Start:      start,
			ErrorTypes: make(map[string]int),
		},
		Start: start,
	}
}

func NewP0dFromFile(f string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg = cfg.validate()

	start := time.Now()
	return &P0d{
		ID:     createRunId(),
		Config: *cfg,
		Client: cfg.scaffoldHttpClient(),
		Stats: &Stats{
			Start:      start,
			ErrorTypes: make(map[string]int),
		},
		Start: time.Now(),
	}
}

func (p *P0d) Race() {
	p.logBootstrap()

	end := make(chan struct{})
	ras := make(chan ReqAtmpt, 65535)

	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		end <- struct{}{}
	})

	for i := 0; i < p.Config.Exec.Threads; i++ {
		go p.doReqAtmpt(ras)
	}

	bar := p.initProgressBar()
Main:
	for {
		select {
		case <-end:
			//just in case the progress bar didn't update to 100% cleanly with the exit signal
			bar.Set(p.Config.Exec.DurationSeconds)

			p.Stop = time.Now()
			p.logSummary(durafmt.Parse(p.Stop.Sub(p.Start)).LimitFirstN(2).String())
			break Main
		case ra := <-ras:
			now := time.Now()
			bar.Set(int(now.Sub(p.Start).Seconds()))

			p.Stats.update(ra, now, p.Config)
		}
	}

}

func (p *P0d) logBootstrap() {
	log.Info().Msgf("%s starting...", p.ID)
	log.Info().Msgf("duration: %s", durafmt.Parse(time.Duration(p.Config.Exec.DurationSeconds)*time.Second).LimitFirstN(2).String())
	log.Info().Msgf("thread(s): %s", FGroup(int64(p.Config.Exec.Threads)))
	log.Info().Msgf("max conn(s): %s", FGroup(int64(p.Config.Exec.Connections)))
	log.Info().Msgf("dial timeout: %s", durafmt.Parse(time.Duration(p.Config.Exec.DialTimeoutSeconds)*time.Second).LimitFirstN(2).String())
	log.Info().Msgf("%s %s", p.Config.Req.Method, p.Config.Req.Url)
}

func (p *P0d) logSummary(elapsed string) {
	//fix issue with progress bar, force newline
	os.Stdout.Write([]byte("\n"))
	log.Info().Msg("")
	log.Info().Msg("|--------------|")
	log.Info().Msg("| Test summary |")
	log.Info().Msg("|--------------|")
	log.Info().Msgf("ID: %s", p.ID)
	log.Info().Msgf("total runtime: %s", elapsed)
	log.Info().Msgf("total HTTP req: %s", FGroup(int64(p.Stats.ReqAtmpts)))
	log.Info().Msgf("mean HTTP req throughput: %s/s", FGroup(int64(p.Stats.ReqAtmptsSec)))
	log.Info().Msgf("mean req latency: %sμs", FGroup(p.Stats.MeanElpsdAtmptLatency.Microseconds()))
	log.Info().Msgf("total bytes read: %s", p.Config.byteCount(p.Stats.SumBytes))
	log.Info().Msgf("mean bytes throughput: %s/s", p.Config.byteCount(int64(p.Stats.MeanBytesSec)))
	log.Info().Msgf("matching HTTP response codes: %s/%s (%s%%)",
		FGroup(int64(p.Stats.SumMatchingResponseCodes)),
		FGroup(int64(p.Stats.ReqAtmpts)),
		fmt.Sprintf("%.2f", p.Stats.PctMatchingResponseCodes))
	log.Info().Msgf("total transport errors: %s/%s (%s%%)",
		FGroup(int64(p.Stats.SumErrors)),
		FGroup(int64(p.Stats.ReqAtmpts)),
		fmt.Sprintf("%.2f", p.Stats.PctErrors))
	for k, v := range p.Stats.ErrorTypes {
		log.Info().Msgf("  - error: [%s]: %s/%s (%s%%)",
			k,
			FGroup(int64(v)),
			FGroup(int64(p.Stats.ReqAtmpts)),
			fmt.Sprintf("%.2f", 100*float32(v)/float32(p.Stats.ReqAtmpts)))
	}

	if p.Stats.SumErrors != 0 {
		os.Exit(-1)
	}
}

func (p *P0d) doReqAtmpt(ras chan<- ReqAtmpt) {
	for {
		ra := ReqAtmpt{
			Start: time.Now(),
		}

		req, _ := http.NewRequest(p.Config.Req.Method,
			p.Config.Req.Url,
			strings.NewReader(p.Config.Req.Body))

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
			b, _ := httputil.DumpResponse(res, true)
			ra.ResponseBytes = int64(len(b))
			_ = b
			res.Body.Close()
		}

		ra.Stop = time.Now()
		ra.Elapsed = ra.Stop.Sub(ra.Start)

		if e != nil {
			em := ""
			for ek, ev := range connectionErrors {
				if strings.Contains(e.Error(), ek) {
					em = ev
				}
			}
			if em == "" {
				em = e.Error()
			}
			ra.ResponseError = em
		}

		req = nil

		ras <- ra
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
