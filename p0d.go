package p0d

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gosuri/uilive"
	"github.com/hako/durafmt"
	. "github.com/logrusorgru/aurora"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Version string = "v0.2.4"

type P0d struct {
	ID             string
	Config         Config
	client         *http.Client
	Stats          *Stats
	Start          time.Time
	Stop           time.Time
	Output         string
	outFile        *os.File
	liveWriters    []io.Writer
	bar            *ProgressBar
	Interrupted    bool
	interrupt      chan os.Signal
	OsMaxOpenFiles int64
}

type ReqAtmpt struct {
	Start    time.Time
	Stop     time.Time
	ElpsdNs  time.Duration
	ReqBytes int64
	ResCode  int
	ResBytes int64
	ResErr   string
}

func NewP0dWithValues(t int, c int, d int, u string, h string, o string) *P0d {
	hv, _ := strconv.ParseFloat(h, 32)

	cfg := Config{
		Req: Req{
			Method: "GET",
			Url:    u,
		},
		Exec: Exec{
			Threads:         t,
			DurationSeconds: d,
			Connections:     c,
			HttpVersion:     float32(hv),
		},
	}
	cfg = *cfg.validate()

	start := time.Now()
	_, ul := getUlimit()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)

	return &P0d{
		ID:     createRunId(),
		Config: cfg,
		client: cfg.scaffoldHttpClient(),
		Stats: &Stats{
			Start:      start,
			ErrorTypes: make(map[string]int),
		},
		Start:          start,
		Output:         o,
		OsMaxOpenFiles: ul,
		interrupt:      sigs,
		Interrupted:    false,
		bar: &ProgressBar{
			maxSecs: d,
			size:    20,
		},
	}
}

func NewP0dFromFile(f string, o string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg.File = f
	cfg = cfg.validate()

	start := time.Now()
	_, ul := getUlimit()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)

	return &P0d{
		ID:     createRunId(),
		Config: *cfg,
		client: cfg.scaffoldHttpClient(),
		Stats: &Stats{
			Start:      start,
			ErrorTypes: make(map[string]int),
		},
		Start:          time.Now(),
		Output:         o,
		OsMaxOpenFiles: ul,
		interrupt:      sigs,
		Interrupted:    false,
		bar: &ProgressBar{
			maxSecs: cfg.Exec.DurationSeconds,
			size:    20,
		},
	}
}

func (p *P0d) Race() {
	_, p.OsMaxOpenFiles = getUlimit()
	p.logBootstrap()

	end := make(chan struct{})
	ras := make(chan ReqAtmpt, 65535)

	//init timer for race to end
	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		end <- struct{}{}
	})

	defer func() {
		if p.outFile != nil {
			p.outFile.Close()
		}
	}()
	p.initOutFile()

	for i := 0; i < p.Config.Exec.Threads; i++ {
		go p.doReqAtmpt(ras)
	}

	p.initLiveWriters(9)

	const prefix string = ""
	const indent string = "  "
	var comma = []byte(",\n")
	const backspace = "\x1b[%dD"
Main:
	for {
		select {
		case <-p.interrupt:
			//because CTRL+C is crazy and messes up our live log by two spaces
			fmt.Fprintf(p.liveWriters[0], backspace, 2)
			p.Stop = time.Now()
			p.Interrupted = true
			p.finaliseOutputAndCloseWriters()
			break Main
		case <-end:
			p.Stop = time.Now()
			p.finaliseOutputAndCloseWriters()
			break Main
		case ra := <-ras:
			p.Stats.update(ra, time.Now(), p.Config)
			p.logRequestAttempt(ra, prefix, indent, comma)
		}
	}

	log(Cyan("done").String())
}

func (p *P0d) doReqAtmpt(ras chan<- ReqAtmpt) {
	for {
		//introduce artifical request latency
		if p.Config.Exec.SpacingMillis > 0 {
			time.Sleep(time.Duration(p.Config.Exec.SpacingMillis) * time.Millisecond)
		}

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

		bq, _ := httputil.DumpRequest(req, true)
		ra.ReqBytes = int64(len(bq))
		_ = bq

		res, e := p.client.Do(req)
		if res != nil {
			ra.ResCode = res.StatusCode
			b, _ := httputil.DumpResponse(res, true)
			ra.ResBytes = int64(len(b))
			_ = b
			res.Body.Close()
		}

		ra.Stop = time.Now()
		ra.ElpsdNs = ra.Stop.Sub(ra.Start)

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
			ra.ResErr = em
		}

		req = nil

		ras <- ra
	}
}

func (p *P0d) finaliseOutputAndCloseWriters() {
	//call final log manually to prevent differences between summary and what's on screen in live log.
	p.logLive()
	p.liveWriters[0].(*uilive.Writer).Stop()
	p.logSummary()

	if len(p.Output) > 0 {
		log("finalizing log file '%s'", Yellow(p.Output))
		j, je := json.MarshalIndent(p, "", "  ")
		p.checkWrite(je)
		_, we := p.outFile.Write(j)
		p.checkWrite(we)
		_, we = p.outFile.Write([]byte("]"))
		p.checkWrite(we)
	}
}

func (p *P0d) logBootstrap() {
	PrintLogo()

	if p.Config.File != "" {
		log("config loaded from '%s'", Yellow(p.Config.File))
	}

	if p.OsMaxOpenFiles == 0 {
		msg := Red(fmt.Sprintf("unable to detect OS open file limit"))
		log("%v", msg)
	} else if p.OsMaxOpenFiles <= int64(p.Config.Exec.Connections) {
		msg := fmt.Sprintf("detected low OS max open file limit %s, reduce connections from %s",
			Red(FGroup(int64(p.OsMaxOpenFiles))),
			Red(FGroup(int64(p.Config.Exec.Connections))))
		log(msg)
	} else {
		ul, _ := getUlimit()
		log("detected OS open file ulimit: %s", ul)
	}
	log("%s starting engines", Cyan(p.ID))
	log("duration: %s",
		Yellow(durafmt.Parse(time.Duration(p.Config.Exec.DurationSeconds)*time.Second).LimitFirstN(2).String()))
	log("preferred http version: %s", Yellow(fmt.Sprintf("%.1f", p.Config.Exec.HttpVersion)))
	log("parallel execution thread(s): %s", Yellow(FGroup(int64(p.Config.Exec.Threads))))
	log("max TCP conn(s): %s", Yellow(FGroup(int64(p.Config.Exec.Connections))))
	log("network dial timeout (inc. TLS handshake): %s",
		Yellow(durafmt.Parse(time.Duration(p.Config.Exec.DialTimeoutSeconds)*time.Second).LimitFirstN(2).String()))
	if p.Config.Exec.SpacingMillis > 0 {
		log("request spacing: %s",
			Yellow(durafmt.Parse(time.Duration(p.Config.Exec.SpacingMillis)*time.Millisecond).LimitFirstN(2).String()))
	}
	if len(p.Output) > 0 {
		log("log sampling rate: %s%s", Yellow(FGroup(int64(100*p.Config.Exec.LogSampling))), Yellow("%"))
	}
	fmt.Printf(timefmt("%s %s"), Yellow(p.Config.Req.Method), Yellow(p.Config.Req.Url))
}

func (p *P0d) logLive() {
	elpsd := time.Now().Sub(p.Start).Seconds()

	lw := p.liveWriters
	fmt.Fprintf(lw[0], timefmt("runtime: %s"), p.bar.render(elpsd, p))

	fmt.Fprintf(lw[1], timefmt("HTTP req: %s"), Cyan(FGroup(int64(p.Stats.ReqAtmpts))))
	fmt.Fprintf(lw[2], timefmt("roundtrip throughput: %s%s"), Cyan(FGroup(int64(p.Stats.ReqAtmptsPSec))), Cyan("/s"))
	fmt.Fprintf(lw[3], timefmt("roundtrip latency: %s%s"), Cyan(FGroup(int64(p.Stats.MeanElpsdAtmptLatencyNs.Milliseconds()))), Cyan("ms"))
	fmt.Fprintf(lw[4], timefmt("bytes read: %s"), Cyan(p.Config.byteCount(p.Stats.SumBytesRead)))
	fmt.Fprintf(lw[5], timefmt("read throughput: %s%s"), Cyan(p.Config.byteCount(int64(p.Stats.MeanBytesReadSec))), Cyan("/s"))
	fmt.Fprintf(lw[6], timefmt("bytes written: %s"), Cyan(p.Config.byteCount(p.Stats.SumBytesWritten)))
	fmt.Fprintf(lw[7], timefmt("write throughput: %s%s"), Cyan(p.Config.byteCount(int64(p.Stats.MeanBytesWrittenSec))), Cyan("/s"))

	mrc := Cyan(fmt.Sprintf("%s/%s (%s%%)",
		FGroup(int64(p.Stats.SumMatchingResponseCodes)),
		FGroup(int64(p.Stats.ReqAtmpts)),
		fmt.Sprintf("%.2f", math.Floor(float64(p.Stats.PctMatchingResponseCodes*100))/100)))
	fmt.Fprintf(lw[8], timefmt("matching HTTP response codes: %v"), mrc)

	tte := fmt.Sprintf("%s/%s (%s%%)",
		FGroup(int64(p.Stats.SumErrors)),
		FGroup(int64(p.Stats.ReqAtmpts)),
		fmt.Sprintf("%.2f", math.Ceil(float64(p.Stats.PctErrors*100))/100))
	if p.Stats.SumErrors > 0 {
		fmt.Fprintf(lw[9], timefmt("transport errors: %v"), Red(tte))
	} else {
		fmt.Fprintf(lw[9], timefmt("transport errors: %v"), Cyan(tte))
	}

	//need to flush manually here to keep stdout updated
	lw[0].(*uilive.Writer).Flush()
}

func (p *P0d) logSummary() {
	for k, v := range p.Stats.ErrorTypes {
		pctv := 100 * (float32(v) / float32(p.Stats.ReqAtmpts))
		err := Red(fmt.Sprintf("  - error: [%s]: %s/%s (%s%%)", k,
			FGroup(int64(v)),
			FGroup(int64(p.Stats.ReqAtmpts)),
			fmt.Sprintf("%.2f", math.Ceil(float64(pctv*100))/100)))
		logv(err)
	}
}

func (p *P0d) logRequestAttempt(ra ReqAtmpt, prefix string, indent string, comma []byte) {
	if len(p.Output) > 0 {
		rand.Seed(time.Now().UnixNano())
		//only sample a subset of requests
		if rand.Float32() < p.Config.Exec.LogSampling {
			j, je := json.MarshalIndent(ra, prefix, indent)
			p.checkWrite(je)
			_, we := p.outFile.Write(j)
			p.checkWrite(we)
			_, we = p.outFile.Write(comma)
			p.checkWrite(we)
		}
	}
}

func (p *P0d) initOutFile() {
	var oe error
	if len(p.Output) > 0 {
		p.outFile, oe = os.Create(p.Output)
		p.checkWrite(oe)
		_, we := p.outFile.Write([]byte("["))
		p.checkWrite(we)
	}
}

func (p *P0d) initLiveWriters(n int) {
	//start live logging

	l0 := uilive.New()
	//this prevents the writer from flushing inbetween lines. we flush manually after each iteration
	l0.RefreshInterval = time.Hour * 24 * 30
	l0.Start()

	live := make([]io.Writer, 0)
	live = append(live, l0)
	for i := 0; i <= n; i++ {
		live = append(live, live[0].(*uilive.Writer).Newline())
	}

	//do this before setting off goroutines
	p.liveWriters = live

	//now start live logging
	go func() {
		for {
			p.logLive()
			time.Sleep(time.Millisecond * 100)
		}
	}()

}

func (p *P0d) checkWrite(e error) {
	if e != nil {
		fmt.Println(e)
		msg := Red(fmt.Sprintf("unable to write to output file %s", p.Output))
		logv(msg)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}
}

func createRunId() string {
	uid, _ := uuid.NewRandom()
	return fmt.Sprintf("p0d-%s-race-%s", Version, uid)
}
