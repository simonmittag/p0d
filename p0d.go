package p0d

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gosuri/uilive"
	"github.com/hako/durafmt"
	. "github.com/logrusorgru/aurora"
	"io"
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Version string = "v0.2.7"
const ua = "User-Agent"
const N = ""
const ct = "Content-Type"
const applicationJson = "application/json"
const multipartFormdata = "multipart/form-data"
const applicationXWWWFormUrlEncoded = "application/x-www-form-urlencoded"
const AT = "@"

var vs = fmt.Sprintf("p0d %s", Version)
var bodyTypes = []string{"POST", "PUT", "PATCH"}

type P0d struct {
	ID              string
	PID             int
	Config          Config
	client          *http.Client
	ReqStats        *ReqStats
	OSStats         []OSStats
	Start           time.Time
	Stop            time.Time
	Output          string
	outFile         *os.File
	liveWriters     []io.Writer
	bar             *ProgressBar
	Interrupted     bool
	interrupt       chan os.Signal
	stopLiveWriters chan struct{}
	stopThreads     []chan struct{}
	OsMaxOpenFiles  int64
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

func initStopThreads(cfg Config) []chan struct{} {
	var v = make([]chan struct{}, 0)
	for i := 0; i < cfg.Exec.Threads; i++ {
		v = append(v, make(chan struct{}))
	}
	return v
}

func NewP0dWithValues(t int, c int, d int, u string, h string, o string) *P0d {
	hv, _ := strconv.ParseFloat(h, 32)

	cfg := Config{
		Req: Req{
			Method:        "GET",
			Url:           u,
			FormData:      make([]map[string]string, 0),
			FormDataFiles: make(map[string][]byte, 0),
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
		ReqStats: &ReqStats{
			Start:      start,
			ErrorTypes: make(map[string]int),
		},
		OSStats:        make([]OSStats, 0),
		Start:          start,
		Output:         o,
		OsMaxOpenFiles: ul,
		interrupt:      sigs,
		Interrupted:    false,
		bar: &ProgressBar{
			maxSecs: d,
			size:    20,
		},
		stopLiveWriters: make(chan struct{}),
		stopThreads:     initStopThreads(cfg),
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
		ReqStats: &ReqStats{
			Start:      start,
			ErrorTypes: make(map[string]int),
		},
		OSStats:        make([]OSStats, 0),
		Start:          time.Now(),
		Output:         o,
		OsMaxOpenFiles: ul,
		interrupt:      sigs,
		Interrupted:    false,
		bar: &ProgressBar{
			maxSecs: cfg.Exec.DurationSeconds,
			size:    20,
		},
		stopLiveWriters: make(chan struct{}),
		stopThreads:     initStopThreads(*cfg),
	}
}

func (p *P0d) Race() {
	_, p.OsMaxOpenFiles = getUlimit()
	p.logBootstrap()

	//init timer for race to end
	end := make(chan struct{})
	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		end <- struct{}{}
	})

	defer func() {
		if p.outFile != nil {
			p.outFile.Close()
		}
	}()

	p.initOutFile()
	p.initOSStats()
	ras := make(chan ReqAtmpt, 65535)
	p.initRequestAtmptThreads(ras)
	p.initLiveWriters(10)

	const prefix string = ""
	const indent string = "  "
	var comma = []byte(",\n")
	const backspace = "\x1b[%dD"

	winddown := func() {
		p.Stop = time.Now()
		p.stopLiveWriter()
		p.stopReqAtmpts()
		//p.logLive()
		//time.Sleep(time.Millisecond * 2000)
		//p.appendOSSStats()
		//p.logLive()
		p.finaliseOutputAndCloseWriters()
	}
Main:
	for {
		select {
		case <-p.interrupt:
			//because CTRL+C is crazy and messes up our live log by two spaces
			fmt.Fprintf(p.liveWriters[0], backspace, 2)
			p.Interrupted = true
			winddown()
			break Main
		case <-end:
			winddown()
			break Main
		case ra := <-ras:
			p.ReqStats.update(ra, time.Now(), p.Config)
			p.logRequestAttempt(ra, prefix, indent, comma)
		}
	}

	log(Cyan("done").String())
}

func (p *P0d) initRequestAtmptThreads(ras chan ReqAtmpt) {
	for i := 0; i < p.Config.Exec.Threads; i++ {
		go p.doReqAtmpt(ras, p.stopThreads[i])
	}
}

func (p *P0d) doReqAtmpt(ras chan<- ReqAtmpt, done <-chan struct{}) {
ReqAtmpt:
	for {
		select {
		case <-done:
			break ReqAtmpt
		default:
		}

		//introduce artifical request latency
		if p.Config.Exec.SpacingMillis > 0 {
			time.Sleep(time.Duration(p.Config.Exec.SpacingMillis) * time.Millisecond)
		}

		ra := ReqAtmpt{
			Start: time.Now(),
		}

		var body io.Reader

		//multipartwriter adds a boundary
		var mpContentType string

		//needs to decide between url encoded, multipart form data and everything else
		switch p.Config.Req.ContentType {
		case applicationXWWWFormUrlEncoded:
			data := url.Values{}
			for _, fd := range p.Config.Req.FormData {
				for k, v := range fd {
					data.Add(k, v)
				}
			}
			body = strings.NewReader(data.Encode())
		case multipartFormdata:
			var b bytes.Buffer
			mpw := multipart.NewWriter(&b)

			for _, fd := range p.Config.Req.FormData {
				for k, v := range fd {
					if strings.HasPrefix(k, AT) {
						fw, _ := mpw.CreateFormFile(k, v)
						mpContentType = mpw.FormDataContentType()
						io.Copy(fw, bytes.NewReader(p.Config.Req.FormDataFiles[k]))
					} else {
						mpw.WriteField(k, v)
					}
				}
			}

			mpw.Close()
			body = bytes.NewReader(b.Bytes())
		case applicationJson:
			fallthrough
		default:
			body = strings.NewReader(p.Config.Req.Body)
		}

		req, _ := http.NewRequest(p.Config.Req.Method,
			p.Config.Req.Url,
			body)

		//set headers from config
		if len(p.Config.Req.Headers) > 0 {
			for _, h := range p.Config.Req.Headers {
				for k, v := range h {
					if k == ct && v == multipartFormdata {
						req.Header.Set(k, mpContentType)
					} else {
						req.Header.Add(k, v)
					}
				}
			}
		}

		//set user agent and default content type to application/json
		req.Header.Set(ua, vs)

		//measure for size before sending. We don't set content length, go does that internally
		bq, _ := httputil.DumpRequest(req, true)
		ra.ReqBytes = int64(len(bq))
		_ = bq

		//do the work and dump the response for size
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

		//report on errors
		if e != nil {
			em := N
			for ek, ev := range connectionErrors {
				if strings.Contains(e.Error(), ek) {
					em = ev
				}
			}
			if em == N {
				em = e.Error()
			}
			ra.ResErr = em
		}

		//null this aggressively
		req = nil

		//and report back
		ras <- ra
	}
}

func (p *P0d) stopLiveWriter() {
	p.stopLiveWriters <- struct{}{}
	close(p.stopLiveWriters)
}

func (p *P0d) stopReqAtmpts() {
	for i := 0; i < len(p.stopThreads); i++ {
		p.stopThreads[i] <- struct{}{}
		close(p.stopThreads[i])
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
	PrintVersion()
	fmt.Printf("\n")
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
	log("parallel thread(s): %s", Yellow(FGroup(int64(p.Config.Exec.Threads))))
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
	i := 0
	fmt.Fprintf(lw[i], timefmt("%s"), p.bar.render(elpsd, p))
	i++
	oss := p.latestOSStats()
	fmt.Fprintf(lw[i], timefmt("TCP open conns: %s%s%s"), Cyan(FGroup(int64(oss.PidOpenConns))), Cyan("/"), Cyan(FGroup(int64(p.Config.Exec.Connections))))
	i++
	fmt.Fprintf(lw[i], timefmt("HTTP req: %s"), Cyan(FGroup(int64(p.ReqStats.ReqAtmpts))))
	i++
	fmt.Fprintf(lw[i], timefmt("roundtrip throughput: %s%s"), Cyan(FGroup(int64(p.ReqStats.ReqAtmptsPSec))), Cyan("/s"))
	i++
	fmt.Fprintf(lw[i], timefmt("roundtrip latency: %s%s"), Cyan(FGroup(int64(p.ReqStats.MeanElpsdAtmptLatencyNs.Milliseconds()))), Cyan("ms"))
	i++
	fmt.Fprintf(lw[i], timefmt("bytes read: %s"), Cyan(p.Config.byteCount(p.ReqStats.SumBytesRead)))
	i++
	fmt.Fprintf(lw[i], timefmt("read throughput: %s%s"), Cyan(p.Config.byteCount(int64(p.ReqStats.MeanBytesReadSec))), Cyan("/s"))
	i++
	fmt.Fprintf(lw[i], timefmt("bytes written: %s"), Cyan(p.Config.byteCount(p.ReqStats.SumBytesWritten)))
	i++
	fmt.Fprintf(lw[i], timefmt("write throughput: %s%s"), Cyan(p.Config.byteCount(int64(p.ReqStats.MeanBytesWrittenSec))), Cyan("/s"))

	mrc := Cyan(fmt.Sprintf("%s/%s (%s%%)",
		FGroup(int64(p.ReqStats.SumMatchingResponseCodes)),
		FGroup(int64(p.ReqStats.ReqAtmpts)),
		fmt.Sprintf("%.2f", math.Floor(float64(p.ReqStats.PctMatchingResponseCodes*100))/100)))
	i++
	fmt.Fprintf(lw[i], timefmt("matching HTTP response codes: %v"), mrc)

	tte := fmt.Sprintf("%s/%s (%s%%)",
		FGroup(int64(p.ReqStats.SumErrors)),
		FGroup(int64(p.ReqStats.ReqAtmpts)),
		fmt.Sprintf("%.2f", math.Ceil(float64(p.ReqStats.PctErrors*100))/100))
	i++
	if p.ReqStats.SumErrors > 0 {
		fmt.Fprintf(lw[i], timefmt("transport errors: %v"), Red(tte))
	} else {
		fmt.Fprintf(lw[i], timefmt("transport errors: %v"), Cyan(tte))
	}

	//need to flush manually here to keep stdout updated
	lw[0].(*uilive.Writer).Flush()
}

func (p *P0d) logSummary() {
	for k, v := range p.ReqStats.ErrorTypes {
		pctv := 100 * (float32(v) / float32(p.ReqStats.ReqAtmpts))
		err := Red(fmt.Sprintf("  - error: [%s]: %s/%s (%s%%)", k,
			FGroup(int64(v)),
			FGroup(int64(p.ReqStats.ReqAtmpts)),
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
	go func(done chan struct{}) {
	LiveWriters:
		for {
			select {
			case <-done:
				break LiveWriters
			default:
				p.logLive()
				time.Sleep(time.Millisecond * 100)
			}
		}
	}(p.stopLiveWriters)
}

func (p *P0d) initOSStats() {
	p.PID = os.Getpid()
	go func() {
		for {
			p.appendOSSStats()
			time.Sleep(time.Millisecond * 250)
		}
	}()
}

func (p *P0d) appendOSSStats() {
	oss := NewOSStats(p.PID)
	oss.updateOpenConns()
	p.OSStats = append(p.OSStats, *oss)
}

func (p *P0d) latestOSStats() OSStats {
	//first time this runs OSS Stats may not have been initialized
	if len(p.OSStats) == 0 {
		return *NewOSStats(os.Getpid())
	} else {
		return p.OSStats[len(p.OSStats)-1]
	}
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
