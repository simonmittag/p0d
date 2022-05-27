package p0d

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gosuri/uilive"
	"github.com/hako/durafmt"
	. "github.com/logrusorgru/aurora"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const Version string = "v0.3.0"
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
	ID               string
	PID              int
	TimerPhase       TimerPhase
	Config           Config
	ReqStats         *ReqStats
	Start            time.Time
	Stop             time.Time
	OSStats          []OSStats
	OSMaxOpenConns   int
	OSLimitOpenFiles int64
	Output           string
	Interrupted      bool

	client          map[int]*http.Client
	sampleConn      net.Conn
	outFile         *os.File
	liveWriters     []io.Writer
	bar             *ProgressBar
	interrupt       chan os.Signal
	stopLiveWriters chan struct{}
	stopThreads     []chan struct{}
}

type TimerPhase int

const (
	Bootstrap TimerPhase = 1 << iota
	RampUp
	Main
	RampDown
	Draining
	Drained
	Done
)

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
	for i := 0; i < cfg.Exec.Concurrency; i++ {
		//make sure this channel never blocks if drain runs after stop
		v = append(v, make(chan struct{}, 2))
	}
	return v
}

func NewP0dWithValues(c int, d int, u string, h string, o string) *P0d {
	hv, _ := strconv.ParseFloat(h, 32)

	cfg := Config{
		Req: Req{
			Method:        "GET",
			Url:           u,
			FormData:      make([]map[string]string, 0),
			FormDataFiles: make(map[string][]byte, 0),
		},
		Exec: Exec{
			DurationSeconds: d,
			Concurrency:     c,
			HttpVersion:     float32(hv),
		},
	}
	cfg = *cfg.validate()

	_, ul := getUlimit()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)

	return &P0d{
		ID:         createRunId(),
		TimerPhase: Bootstrap,
		Config:     cfg,
		client:     cfg.scaffoldHttpClients(),
		ReqStats: &ReqStats{
			ErrorTypes: make(map[string]int),
			Sample:     NewSample(),
		},
		OSStats:          make([]OSStats, 0),
		Output:           o,
		OSLimitOpenFiles: ul,
		OSMaxOpenConns:   0,
		interrupt:        sigs,
		Interrupted:      false,
		bar: &ProgressBar{
			maxSecs:    d,
			size:       30,
			chunkProps: make([]ChunkProps, 30),
		},
		stopLiveWriters: make(chan struct{}),
		stopThreads:     initStopThreads(cfg),
	}
}

func NewP0dFromFile(f string, o string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg.File = f
	cfg = cfg.validate()

	_, ul := getUlimit()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)

	return &P0d{
		ID:         createRunId(),
		TimerPhase: Bootstrap,
		Config:     *cfg,
		client:     cfg.scaffoldHttpClients(),
		ReqStats: &ReqStats{
			ErrorTypes: make(map[string]int),
			Sample:     NewSample(),
		},
		OSStats:          make([]OSStats, 0),
		Output:           o,
		OSLimitOpenFiles: ul,
		OSMaxOpenConns:   0,
		interrupt:        sigs,
		Interrupted:      false,
		bar: &ProgressBar{
			maxSecs:    cfg.Exec.DurationSeconds,
			size:       30,
			chunkProps: make([]ChunkProps, 30),
		},
		stopLiveWriters: make(chan struct{}),
		stopThreads:     initStopThreads(*cfg),
	}
}

func (p *P0d) StartTimeNow() {
	now := time.Now()
	p.Start = now
	p.ReqStats.Start = now
}

func (p *P0d) Race() {
	_, p.OSLimitOpenFiles = getUlimit()
	p.detectRemoteConnSettings()
	p.initLog()

	defer func() {
		if p.outFile != nil {
			p.outFile.Close()
		}
	}()
	p.initOutFile()

	p.StartTimeNow()
	p.bar.updateRampStateForTimerPhase(p.Start, p)

	osStatsDone := make(chan struct{})
	p.initOSStats(osStatsDone)

	//init timer for rampdown trigger
	rampdown := make(chan struct{})
	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds-p.Config.Exec.RampSeconds)*time.Second, func() {
		rampdown <- struct{}{}
	})

	//init timer for trigger end to totalruntime and start draining
	drainer := make(chan struct{})
	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		drainer <- struct{}{}
	})

	//init req attempts loop
	ras := make(chan ReqAtmpt, 65535)

	//this done channel is buffered because it may be too late to signal. we don't want to block
	initReqAtmptsDone := make(chan struct{}, 2)
	p.initReqAtmpts(initReqAtmptsDone, ras)

	p.initLiveWriterFastLoop(10)

	const prefix string = ""
	const indent string = "  "
	var comma = []byte(",\n")
	const backspace = "\x1b[%dD"

	drain := func() {
		//this one log event renders the progress bar at 0 seconds remaining
		initReqAtmptsDone <- struct{}{}
		p.doLogLive()
		p.Stop = time.Now()
		p.setTimerPhase(Draining)
		//we still want to watch draining but much faster.
		p.stopReqAtmptsThreads(time.Millisecond * 1)
		p.stopLiveWriterFastLoop()
	Drain:
		for i := 0; i < 300; i++ {
			if p.getOSStats().OpenConns == 0 {
				break Drain
			}
			time.Sleep(time.Millisecond * 100)
			p.doLogLive()
		}
		p.setTimerPhase(Drained)
		p.closeLiveWritersAndSummarize()
	}
Main:
	for {
		select {
		case <-p.interrupt:
			//because CTRL+C is crazy and messes up our live log by two spaces
			fmt.Fprintf(p.liveWriters[0], backspace, 2)
			p.Interrupted = true
			drain()
			break Main
		case <-drainer:
			drain()
			break Main
		case <-rampdown:
			p.setTimerPhase(RampDown)
			p.stopReqAtmptsThreads(p.staggerThreadsDuration())
		case ra := <-ras: //sets timerphase for when this is read, not at the time when request was made.
			p.ReqStats.update(ra, ra.Stop, p.Config)
			p.outFileRequestAttempt(ra, prefix, indent, comma)
		}
	}
	p.setTimerPhase(Done)

	osStatsDone <- struct{}{}
	log(Cyan("exiting").String())
}

const defMsg = "not detected"

func (p *P0d) detectRemoteConnSettings() {
	c := p.Config.scaffoldHttpClientWith(1, true, p)
	r := p.scaffoldHttpReq()

	rr, e := c.Do(r)
	if e == nil {
		p.ReqStats.Sample.Server = rr.Header.Get("Server")

		io.Copy(ioutil.Discard, rr.Body)
		defer rr.Body.Close()
		p.ReqStats.Sample.HTTPVersion = fmt.Sprintf("HTTP/%d.%d", rr.ProtoMajor, rr.ProtoMinor)

		if rr.TLS != nil {
			if rr.TLS.Version == tls.VersionSSL30 {
				p.ReqStats.Sample.TLSVersion = "SSL3.0"
			} else if rr.TLS.Version == tls.VersionTLS10 {
				p.ReqStats.Sample.TLSVersion = "TLS1.0"
			} else if rr.TLS.Version == tls.VersionTLS11 {
				p.ReqStats.Sample.TLSVersion = "TLS1.1"
			} else if rr.TLS.Version == tls.VersionTLS12 {
				p.ReqStats.Sample.TLSVersion = "TLS1.2"
			} else if rr.TLS.Version == tls.VersionTLS13 {
				p.ReqStats.Sample.TLSVersion = "TLS1.3"
			}
		}

		if p.sampleConn != nil {
			addr, _, _ := net.SplitHostPort(p.sampleConn.RemoteAddr().String())
			ip4 := net.ParseIP(addr).To4()
			if ip4 != nil {
				p.ReqStats.Sample.IPVersion = "IPV4"
			} else {
				p.ReqStats.Sample.IPVersion = "IPV6"
			}
			p.ReqStats.Sample.RemoteAddr = addr
			p.sampleConn.Close()
		}
	}
}

func (p *P0d) initReqAtmpts(done chan struct{}, ras chan ReqAtmpt) {

	//don't block because execution continues on to live updates
	go func() {
		bd := false
		p.setTimerPhase(RampUp)
	RampUp:
		for i := 0; i < p.Config.Exec.Concurrency; i++ {
			select {
			case <-done:
				bd = true
				break RampUp
			default:
				//stagger the initialisation so we can watch ramp up live.
				go p.doReqAtmpts(i, ras, p.stopThreads[i])
				if p.Config.Exec.Concurrency > 1 && i < p.Config.Exec.Concurrency-1 {
					time.Sleep(p.staggerThreadsDuration())
				}
			}
		}

		//we don't want to run this if we aborted above
		if !bd && p.TimerPhase < Main {
		MainUpdate:
			for {
				if p.getOSStats().OpenConns >= p.Config.Exec.Concurrency {
					p.setTimerPhase(Main)
					break MainUpdate
				}
				time.Sleep(time.Millisecond * 100)
			}
		}
	}()
}

func (p *P0d) staggerThreadsDuration() time.Duration {
	return time.Duration(
		float64(time.Second) * (float64(p.Config.Exec.RampSeconds) / float64(p.Config.Exec.Concurrency)),
	)
}

func (p *P0d) doReqAtmpts(i int, ras chan<- ReqAtmpt, done <-chan struct{}) {
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
		p.bar.updateRampStateForTimerPhase(ra.Start, p)

		req := p.scaffoldHttpReq()

		//measure for size before sending. We don't set content length, go does that internally
		bq, _ := httputil.DumpRequest(req, true)
		ra.ReqBytes = int64(len(bq))
		_ = bq

		//do the work and dump the response for size
		res, e := p.client[i].Do(req)
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

		if len(ra.ResErr) > 0 {
			p.bar.markError(ra.Stop, p)
		}

		//null this aggressively
		req = nil

		//and report back
		ras <- ra
	}
}

func (p *P0d) scaffoldHttpReq() *http.Request {
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

	//set user agent
	req.Header.Set(ua, vs)

	return req
}

func (p *P0d) stopReqAtmptsThreads(staggerThreadsDuration time.Duration) {
	//again don't block because execution continues on with live udpates
	go func() {
		for i := 0; i < len(p.stopThreads); i++ {
			if p.stopThreads[i] != nil {
				//stagger the off ramp between threads so we can watch it live.
				if staggerThreadsDuration > 0 {
					time.Sleep(staggerThreadsDuration)
				}
				p.stopThreads[i] <- struct{}{}
			}
		}
	}()
}

func (p *P0d) initLiveWriterFastLoop(n int) {
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
				p.doLogLive()
				time.Sleep(time.Millisecond * 100)
			}
		}
	}(p.stopLiveWriters)
}

func (p *P0d) stopLiveWriterFastLoop() {
	p.stopLiveWriters <- struct{}{}
	close(p.stopLiveWriters)
}

func (p *P0d) closeLiveWritersAndSummarize() {
	//call final log manually to prevent differences between summary and what's on screen in live log.
	p.doLogLive()
	p.liveWriters[0].(*uilive.Writer).Stop()
	p.logSummary()

	if len(p.Output) > 0 {
		log("finalizing out file '%s'", Yellow(p.Output))
		j, je := json.MarshalIndent(p, "", "  ")
		p.outFileCheckWrite(je)
		_, we := p.outFile.Write(j)
		p.outFileCheckWrite(we)
		_, we = p.outFile.Write([]byte("]"))
		p.outFileCheckWrite(we)
	}
}

func (p *P0d) initLog() {
	PrintLogo()
	PrintVersion()
	fmt.Printf("\n")
	if p.Config.File != "" {
		slog("config loaded from '%s'", Yellow(p.Config.File))
	}

	if p.OSLimitOpenFiles == 0 {
		msg := Red(fmt.Sprintf("unable to detect OS open file limit"))
		slog("%v", msg)
	} else if p.OSLimitOpenFiles <= int64(p.Config.Exec.Concurrency) {
		msg := fmt.Sprintf("detected low OS open file limit %s, reduce concurrency from %s",
			Red(FGroup(int64(p.OSLimitOpenFiles))),
			Red(FGroup(int64(p.Config.Exec.Concurrency))))
		slog(msg)
	} else {
		ul, _ := getUlimit()
		slog("detected local OS open file ulimit: %s", ul)
	}
	time.Sleep(time.Millisecond * 200)

	slog("configured duration: %s",
		Yellow(durafmt.Parse(time.Duration(p.Config.Exec.DurationSeconds)*time.Second).LimitFirstN(2).String()))

	slog("configured max concurrent TCP conn(s): %s", Yellow(FGroup(int64(p.Config.Exec.Concurrency))))
	slog("configured network dial timeout (inc. TLS handshake): %s",
		Yellow(durafmt.Parse(time.Duration(p.Config.Exec.DialTimeoutSeconds)*time.Second).LimitFirstN(2).String()))
	if p.Config.Exec.SpacingMillis > 0 {
		slog("configured request spacing: %s",
			Yellow(durafmt.Parse(time.Duration(p.Config.Exec.SpacingMillis)*time.Millisecond).LimitFirstN(2).String()))
	}
	if len(p.Output) > 0 {
		slog("configured out file sampling rate: %s%s", Yellow(FGroup(int64(100*p.Config.Exec.LogSampling))), Yellow("%"))
	}
	slog("configured preferred http version: %s ",
		Yellow(fmt.Sprintf("%.1f", p.Config.Exec.HttpVersion)),
	)
	fmt.Printf(timefmt("%s %s"), Yellow(p.Config.Req.Method), Yellow(p.Config.Req.Url))

	tv := ""
	if p.ReqStats.Sample.TLSVersion == defMsg {
		tv = N
	} else {
		tv = p.ReqStats.Sample.TLSVersion
	}

	sv := ""
	if p.ReqStats.Sample.Server == defMsg {
		sv = N
	} else {
		sv = p.ReqStats.Sample.Server + " "
	}
	slog("sampled remote conn settings: %s%s%s%s%s %s %s",
		Cyan(sv),
		Cyan(p.ReqStats.Sample.RemoteAddr),
		Cyan("["),
		Cyan(p.ReqStats.Sample.IPVersion),
		Cyan("]"),
		Cyan(p.ReqStats.Sample.HTTPVersion),
		Cyan(tv),
	)

	slog("%s starting engines", Cyan(p.ID))
}

var logLiveLock = sync.Mutex{}

const conMsg = "concurrent TCP conns: %s%s%s"

var rampingUp = Cyan(" (ramping up)").String()
var rampingDown = Cyan(" (ramping down)").String()
var draining = Cyan(" (draining) ").String()
var drained = Cyan(" (drained)").String()

const httpReqSMsg = "HTTP req: %s"
const readThroughputMsg = "roundtrip throughput: %s%s max: %s%s"
const meanRoundTripLatency = "mean roundtrip latency: %s%s"
const bytesReadMsg = "bytes read: %s"
const readthroughputMsg = "read throughput: %s%s max: %s%s"
const perSecondMsg = "/s"
const bytesWrittenMsg = "bytes written: %s"
const writeThroughputMsg = "write throughput: %s%s max: %s%s"
const matchingResponseCodesMsg = "matching HTTP response codes: %v"
const transportErrorsMsg = "transport errors: %v"
const maxMsg = " max: "

func (p *P0d) doLogLive() {
	logLiveLock.Lock()
	elpsd := time.Now()

	lw := p.liveWriters
	i := 0
	fmt.Fprintf(lw[i], timefmt("%s"), p.bar.render(elpsd, p))

	i++
	oss := p.getOSStats()

	connMsg := conMsg
	if p.isTimerPhase(RampUp) {
		connMsg += rampingUp
	} else if p.isTimerPhase(Main) {
		//nothing here
	} else if p.isTimerPhase(RampDown) {
		connMsg += rampingDown
	} else if p.isTimerPhase(Draining) {
		connMsg += draining
	} else if p.isTimerPhase(Drained) {
		connMsg += drained
	}

	connMsg += maxMsg
	connMsg += Magenta(FGroup(int64(p.OSMaxOpenConns))).String()

	fmt.Fprintf(lw[i], timefmt(connMsg),
		Cyan(FGroup(int64(oss.OpenConns))),
		Cyan("/"),
		Cyan(FGroup(int64(p.Config.Exec.Concurrency))))

	i++

	fmt.Fprintf(lw[i], timefmt(httpReqSMsg),
		Cyan(FGroup(int64(p.ReqStats.ReqAtmpts))))

	i++

	fmt.Fprintf(lw[i], timefmt(readThroughputMsg),
		Cyan(FGroup(int64(p.ReqStats.ReqAtmptsPSec))),
		Cyan(perSecondMsg),
		Magenta(FGroup(int64(p.ReqStats.MaxReqAtmptsPSec))),
		Magenta(perSecondMsg))

	i++

	fmt.Fprintf(lw[i], timefmt(meanRoundTripLatency),
		Cyan(FGroup(int64(p.ReqStats.MeanElpsdAtmptLatencyNs.Milliseconds()))),
		Cyan("ms"))

	i++
	fmt.Fprintf(lw[i], timefmt(bytesReadMsg),
		Cyan(p.Config.byteCount(p.ReqStats.SumBytesRead)))

	i++

	fmt.Fprintf(lw[i], timefmt(readthroughputMsg),
		Cyan(p.Config.byteCount(int64(p.ReqStats.MeanBytesReadSec))),
		Cyan(perSecondMsg),
		Magenta(p.Config.byteCount(int64(p.ReqStats.MaxBytesReadSec))),
		Magenta(perSecondMsg))

	i++

	fmt.Fprintf(lw[i], timefmt(bytesWrittenMsg),
		Cyan(p.Config.byteCount(p.ReqStats.SumBytesWritten)))
	i++

	fmt.Fprintf(lw[i], timefmt(writeThroughputMsg),
		Cyan(p.Config.byteCount(int64(p.ReqStats.MeanBytesWrittenSec))),
		Cyan(perSecondMsg),
		Magenta(p.Config.byteCount(int64(p.ReqStats.MaxBytesWrittenSec))),
		Magenta(perSecondMsg))

	i++
	mrc := Cyan(fmt.Sprintf("%s (%s%%)",
		FGroup(int64(p.ReqStats.SumMatchingResponseCodes)),
		fmt.Sprintf("%.2f", math.Floor(float64(p.ReqStats.PctMatchingResponseCodes*100))/100)))

	fmt.Fprintf(lw[i], timefmt(matchingResponseCodesMsg), mrc)

	i++
	tte := fmt.Sprintf("%s (%s%%)",
		FGroup(int64(p.ReqStats.SumErrors)),
		fmt.Sprintf("%.2f", math.Ceil(float64(p.ReqStats.PctErrors*100))/100))

	if p.ReqStats.SumErrors > 0 {
		fmt.Fprintf(lw[i], timefmt(transportErrorsMsg), Red(tte))
	} else {
		fmt.Fprintf(lw[i], timefmt(transportErrorsMsg), Cyan(tte))
	}

	//need to flush manually here to keep stdout updated
	lw[0].(*uilive.Writer).Flush()
	logLiveLock.Unlock()
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

func (p *P0d) initOutFile() {
	var oe error
	if len(p.Output) > 0 {
		p.outFile, oe = os.Create(p.Output)
		p.outFileCheckWrite(oe)
		_, we := p.outFile.Write([]byte("["))
		p.outFileCheckWrite(we)
	}
}

func (p *P0d) outFileCheckWrite(e error) {
	if e != nil {
		fmt.Println(e)
		msg := Red(fmt.Sprintf("unable to write to output file %s", p.Output))
		logv(msg)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}
}

func (p *P0d) outFileRequestAttempt(ra ReqAtmpt, prefix string, indent string, comma []byte) {
	if len(p.Output) > 0 {
		rand.Seed(time.Now().UnixNano())
		//only sample a subset of requests
		if rand.Float32() < p.Config.Exec.LogSampling {
			j, je := json.MarshalIndent(ra, prefix, indent)
			p.outFileCheckWrite(je)
			_, we := p.outFile.Write(j)
			p.outFileCheckWrite(we)
			_, we = p.outFile.Write(comma)
			p.outFileCheckWrite(we)
		}
	}
}

func (p *P0d) initOSStats(done chan struct{}) {
	p.PID = os.Getpid()
	go func() {
	OSStats:
		for {
			select {
			case <-done:
				break OSStats
			default:
				p.doOSSStats()
				time.Sleep(time.Millisecond * 100)
			}
		}
	}()
}

var osMutex = sync.Mutex{}

func (p *P0d) doOSSStats() {
	osMutex.Lock()
	oss := NewOSStats(p.PID)
	oss.updateOpenConns(p.Config)
	p.OSStats = append(p.OSStats, *oss)
	if oss.OpenConns > p.OSMaxOpenConns {
		p.OSMaxOpenConns = oss.OpenConns
	}
	osMutex.Unlock()
}

func (p *P0d) getOSStats() OSStats {
	//first time this runs OSS Stats may not have been initialized
	if len(p.OSStats) == 0 {
		oss := *NewOSStats(os.Getpid())
		oss.OpenConns = 0
		return oss
	} else {
		return p.OSStats[len(p.OSStats)-1]
	}
}

func (p *P0d) setTimerPhase(phase TimerPhase) {
	if phase > p.TimerPhase {
		p.TimerPhase = phase
	}
}

func (p *P0d) isTimerPhase(phase TimerPhase) bool {
	return p.TimerPhase == phase
}

func createRunId() string {
	uid, _ := uuid.NewRandom()
	return fmt.Sprintf("p0d-%s-race-%s", Version, uid)
}
