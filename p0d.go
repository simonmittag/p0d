package p0d

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/axiomhq/variance"
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
	"sync/atomic"
	"syscall"
	"time"
)

const Version string = "v0.3.8"
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
	ID          string
	Time        Time
	Config      Config
	OS          OS
	ReqStats    *ReqStats
	Output      string
	Interrupted bool

	client          map[int]*http.Client
	sampleConn      net.Conn
	outFile         *os.File
	liveWriters     []io.Writer
	bar             *ProgressBar
	interrupt       chan os.Signal
	stopLiveWriters chan struct{}
	stopThreads     []chan struct{}
}

type Time struct {
	Start time.Time
	Stop  time.Time
	Phase TimerPhase
}

type OS struct {
	PID              int
	OpenConns        []OSOpenConns
	MaxOpenConns     int
	LimitOpenFiles   int64
	LimitRAMBytes    uint64
	InetLatencyNs    time.Duration
	InetUlSpeedMBits float64
	InetDlSpeedMBits float64
	InetTestAborted  bool

	inetUlSpeedDoneFlag bool
	inetDlSpeedDoneFlag bool
	inetLatencyDoneFlag bool
	inetUlSpeedDone     chan struct{}
	inetDlSpeedDone     chan struct{}
	inetLatencyDone     chan struct{}
	inetTestError       chan struct{}
	updateLock          sync.Mutex
}

func (o *OS) isInetTestDone() bool {
	return o.inetDlSpeedDoneFlag && o.inetUlSpeedDoneFlag && o.inetLatencyDoneFlag
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

func interruptChannel() chan os.Signal {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)
	return sigs
}

func NewP0dWithValues(c int, d int, u string, h string, o string, s bool) *P0d {
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
			SkipInetTest:    s,
		},
	}
	cfg = *cfg.validate()

	_, ul := getUlimit()

	return NewP0d(cfg, ul, o, d, interruptChannel())
}

func NewP0dFromFile(f string, o string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg.File = f
	cfg = cfg.validate()

	_, ul := getUlimit()

	return NewP0d(*cfg, ul, o, cfg.Exec.DurationSeconds, interruptChannel())
}

func NewP0d(cfg Config, ulimit int64, outputFile string, durationSecs int, interrupt chan os.Signal) *P0d {
	return &P0d{
		ID: createRunId(),
		Time: Time{
			Phase: Bootstrap,
		},
		Config: cfg,
		OS: OS{
			OpenConns:       make([]OSOpenConns, 0),
			LimitOpenFiles:  ulimit,
			MaxOpenConns:    0,
			updateLock:      sync.Mutex{},
			InetTestAborted: false,
			inetUlSpeedDone: make(chan struct{}),
			inetDlSpeedDone: make(chan struct{}),
			inetLatencyDone: make(chan struct{}),
			inetTestError:   make(chan struct{}),
		},
		ReqStats: &ReqStats{
			ErrorTypes:                   make(map[string]int),
			Sample:                       NewSample(),
			ElpsdAtmptLatencyNsQuantiles: NewQuantileWithCompression(500),
			ElpsdAtmptLatencyNs:          &Welford{s: variance.New()},
		},
		Output:      outputFile,
		Interrupted: false,

		client: cfg.scaffoldHttpClients(),
		bar: &ProgressBar{
			maxSecs:    durationSecs,
			size:       30,
			chunkProps: make([]ChunkProps, 30),
		},
		interrupt:       interrupt,
		stopLiveWriters: make(chan struct{}),
		stopThreads:     initStopThreads(cfg),
	}
}

func (p *P0d) StartTimeNow() {
	now := time.Now()
	p.Time.Start = now
	p.ReqStats.Start = now
}

const backspace = "\x1b[%dD"

func (p *P0d) Race() {
	osStatsDone := make(chan struct{}, 2)
	p.initOSStats(osStatsDone)
	p.detectRemoteConnSettings()
	p.initLog()

	defer func() {
		if p.outFile != nil {
			p.outFile.Close()
		}
	}()
	p.initOutFile()

	p.StartTimeNow()
	p.bar.updateRampStateForTimerPhase(p.Time.Start, p)

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

	if !p.Interrupted {
		//this done channel is buffered because it may be too late to signal. we don't want to block
		initReqAtmptsDone := make(chan struct{}, 2)
		p.initReqAtmpts(initReqAtmptsDone, ras)

		p.initLiveWriterFastLoop(8)

		const prefix string = ""
		const indent string = "  "
		var comma = []byte(",\n")

		drain := func() {
			//this one log event renders the progress bar at 0 seconds remaining
			initReqAtmptsDone <- struct{}{}
			p.doLogLive()
			p.Time.Stop = time.Now()
			p.setTimerPhase(Draining)
			//we still want to watch draining but much faster.
			p.stopReqAtmptsThreads(time.Millisecond * 1)
			p.stopLiveWriterFastLoop()
		Drain:
			for i := 0; i < 300; i++ {
				if p.getOSOpenConns().OpenConns == 0 {
					osStatsDone <- struct{}{}
					break Drain
				}
				time.Sleep(time.Millisecond * 100)
				p.doLogLive()
			}
			p.setTimerPhase(Drained)
			//do this so no cur atmpts continue to be reported and all remaining decrease timers fire
			time.Sleep(time.Millisecond * 1010)
			atomic.StoreInt64(&p.ReqStats.CurReqAtmptsPSec, 0)
			p.closeLiveWritersAndSummarize()
		}
	Main:
		for {
			select {
			case <-p.interrupt:
				//because CTRL+C is crazy and messes up our live log by two spaces
				fmt.Fprintf(p.liveWriters[0], backspace, 2)
				p.Interrupted = true
				//in case of interupt we signal the inet speed test to cancel if it's still running
				if !p.Config.Exec.SkipInetTest && !p.OS.isInetTestDone() {
					p.OS.inetTestError <- struct{}{}
				}
				drain()
				break Main
			case <-drainer:
				drain()
				break Main
			case <-rampdown:
				p.setTimerPhase(RampDown)
				p.stopReqAtmptsThreads(p.staggerThreadsDuration())
			case ra := <-ras:
				p.ReqStats.update(ra, ra.Stop, p.Config)
				p.outFileRequestAttempt(ra, prefix, indent, comma)
			}
		}
	}
	p.setTimerPhase(Done)

	osStatsDone <- struct{}{}
	//adjust time stop for aborts
	p.Time.Stop = time.Now()
	p.finalizeOutFile()
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
	c.CloseIdleConnections()
	c = nil
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
		if !bd && p.Time.Phase < Main {
		MainUpdate:
			for {
				if p.getOSOpenConns().OpenConns >= p.Config.Exec.Concurrency {
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
		Mapping:
			for ek, ev := range errorMapping {
				if strings.Contains(e.Error(), ek) {
					em = ev
					break Mapping
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
}

func (p *P0d) finalizeOutFile() {
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

	b128k := 2 << 16
	wantRamBytes := uint64(p.Config.Exec.Concurrency * b128k)
	ramUsagePct := (float32(wantRamBytes) / float32(p.OS.LimitRAMBytes)) * 100

	var ramUsagePctPrec string
	if ramUsagePct < 0.01 {
		ramUsagePctPrec = "%.4f"
	} else {
		ramUsagePctPrec = "%.2f"
	}
	if p.OS.LimitRAMBytes == 0 {
		msg := Red(fmt.Sprintf("unable to detect OS RAM"))
		slog("%v", msg)
	} else if p.OS.LimitRAMBytes < wantRamBytes {
		msg := fmt.Sprintf("detected low OS RAM %s, increase to %s or reduce concurrency from %s",
			Red(p.Config.byteCount(int64(p.OS.LimitRAMBytes))),
			Red(p.Config.byteCount(int64(wantRamBytes))),
			Red(FGroup(int64(p.Config.Exec.Concurrency))))
		slog(msg)
	} else {
		slog("detected OS RAM: %s predicted use max %s %s",
			Yellow(p.Config.byteCount(int64(p.OS.LimitRAMBytes))),
			Yellow(p.Config.byteCount(int64(wantRamBytes))),
			Yellow("("+fmt.Sprintf(ramUsagePctPrec, ramUsagePct)+"%)"))
	}

	if p.OS.LimitOpenFiles == 0 {
		msg := Red(fmt.Sprintf("unable to detect OS open file limit"))
		slog("%v", msg)
	} else if p.OS.LimitOpenFiles <= int64(p.Config.Exec.Concurrency) {
		msg := fmt.Sprintf("detected low OS open file limit %s, reduce concurrency from %s",
			Red(FGroup(int64(p.OS.LimitOpenFiles))),
			Red(FGroup(int64(p.Config.Exec.Concurrency))))
		slog(msg)
	} else {
		ul, _ := getUlimit()
		slog("detected local OS open file ulimit: %s", ul)
	}

	if !p.Config.Exec.SkipInetTest {
		var unable = Red(fmt.Sprintf("unable to detect inet speed")).String()
		uidelay := func() {
			time.Sleep(time.Duration(100) * time.Millisecond)
		}

		if p.OS.InetTestAborted {
			msg := unable
			slog("%v", msg)
		} else {
			w := uilive.New()
			w.RefreshInterval = time.Hour * 24 * 30
			w.Start()
			msg := "detecting inet ▼️ speed"
			b := NewSpinnerAnim()
		OSNet:
			for {
				select {
				case <-p.interrupt:
					msg = unable
					fmt.Fprintf(w, backspace, 2)
					w.Write([]byte(timefmt(msg)))
					w.Flush()
					p.OS.InetTestAborted = true
					p.Interrupted = true
					break OSNet
				case <-p.OS.inetTestError:
					msg = unable
					w.Write([]byte(timefmt(msg)))
					uidelay()
					break OSNet
				case <-p.OS.inetLatencyDone:
					msg = fmt.Sprintf("detected inet ▼️ speed %s%s, ▲ speed %s%s, latency %s",
						Yellow(fmt.Sprintf("%.2f", p.OS.InetDlSpeedMBits)),
						Yellow("MBit/s"),
						Yellow(fmt.Sprintf("%.2f", p.OS.InetUlSpeedMBits)),
						Yellow("MBit/s"),
						Yellow(durafmt.Parse(p.OS.InetLatencyNs).LimitFirstN(1).String()))
					w.Write([]byte(timefmt(msg)))
					uidelay()
					break OSNet
				case <-p.OS.inetUlSpeedDone:
					msg = fmt.Sprintf("detected inet ▼️ speed %s%s, ▲ speed %s%s, detecting latency",
						Yellow(fmt.Sprintf("%.2f", p.OS.InetDlSpeedMBits)),
						Yellow("MBit/s"),
						Yellow(fmt.Sprintf("%.2f", p.OS.InetUlSpeedMBits)),
						Yellow("MBit/s"))
					w.Write([]byte(timefmt(msg)))
					uidelay()
				case <-p.OS.inetDlSpeedDone:
					msg = fmt.Sprintf("detected inet ▼️ speed %s%s, detecting ▲ speed",
						Yellow(fmt.Sprintf("%.2f", p.OS.InetDlSpeedMBits)),
						Yellow("MBit/s"))
					w.Write([]byte(timefmt(msg)))
					uidelay()
				default:
					w.Write([]byte(timefmt(msg + b.Next())))
				}
				w.Flush()
				uidelay()
			}
			w.Stop()
		}
	}

	slog("set test duration: %s",
		Yellow(durafmt.Parse(time.Duration(p.Config.Exec.DurationSeconds)*time.Second).LimitFirstN(2).String()))

	slog("set max concurrent TCP conn(s): %s", Yellow(FGroup(int64(p.Config.Exec.Concurrency))))
	slog("set network dial timeout (inc. TLS handshake): %s",
		Yellow(durafmt.Parse(time.Duration(p.Config.Exec.DialTimeoutSeconds)*time.Second).LimitFirstN(2).String()))
	if p.Config.Exec.SpacingMillis > 0 {
		slog("set request spacing: %s",
			Yellow(durafmt.Parse(time.Duration(p.Config.Exec.SpacingMillis)*time.Millisecond).LimitFirstN(2).String()))
	}
	if len(p.Output) > 0 {
		slog("set out file sampling rate: %s",
			Yellow(strconv.FormatFloat(float64(p.Config.Exec.LogSampling), 'f', -1, 64)))
	}
	slog("set preferred http version: %s ",
		Yellow(fmt.Sprintf("%.1f", p.Config.Exec.HttpVersion)),
	)
	fmt.Printf(timefmt("set URL %s (%s)"), Yellow(p.Config.Req.Url), Yellow(p.Config.Req.Method))

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
	slog("detected remote conn settings: %s%s%s%s%s %s %s",
		Cyan(sv),
		Cyan(p.ReqStats.Sample.RemoteAddr),
		Cyan("["),
		Cyan(p.ReqStats.Sample.IPVersion),
		Cyan("]"),
		Cyan(p.ReqStats.Sample.HTTPVersion),
		Cyan(tv),
	)

	slog("starting engines: %v", Cyan(p.ID))
}

var logLiveLock = sync.Mutex{}

const conMsg = "concurrent TCP conns: %s%s%s"

var rampingUp = Cyan(" (ramping up)").String()
var rampingDown = Cyan(" (ramping down)").String()
var draining = Cyan(" (draining) ").String()
var drained = Cyan(" (drained)").String()

const httpReqSMsg = "HTTP req: %s"
const roundtripThroughputMsg = "roundtrip throughput: %s%s mean: %s%s max: %s%s"
const pctRoundTripLatency = "roundtrip latency pct10: %s pct50: %s pct90: %s pct99: %s"
const readthroughputMsg = "read throughput: %s%s mean: %s%s max: %s%s sum: %s"
const writeThroughputMsg = "write throughput: %s%s mean: %s%s max: %s%s sum: %s"
const matchingResponseCodesMsg = "matching HTTP response codes: %v"
const transportErrorsMsg = "transport errors: %v"
const maxMsg = " max: "
const perSecondMsg = "/s"

func (p *P0d) doLogLive() {
	logLiveLock.Lock()
	elpsd := time.Now()

	lw := p.liveWriters
	i := 0
	fmt.Fprintf(lw[i], timefmt("%s"), p.bar.render(elpsd, p))

	i++
	oss := p.getOSOpenConns()

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
	connMsg += Magenta(FGroup(int64(p.OS.MaxOpenConns))).String()

	fmt.Fprintf(lw[i], timefmt(connMsg),
		Cyan(FGroup(int64(oss.OpenConns))),
		Cyan("/"),
		Cyan(FGroup(int64(p.Config.Exec.Concurrency))))

	i++

	fmt.Fprintf(lw[i], timefmt(httpReqSMsg),
		Cyan(FGroup(int64(p.ReqStats.ReqAtmpts))))

	i++

	fmt.Fprintf(lw[i], timefmt(roundtripThroughputMsg),
		Cyan(FGroup(int64(p.ReqStats.CurReqAtmptsPSec))),
		Cyan(perSecondMsg),
		Cyan(FGroup(int64(p.ReqStats.MeanReqAtmptsPSec))),
		Cyan(perSecondMsg),
		Magenta(FGroup(int64(p.ReqStats.MaxReqAtmptsPSec))),
		Magenta(perSecondMsg))

	i++

	convertToMs := func(q *Quantile, v float64) string {
		qv := q.Quantile(v)
		if math.IsNaN(qv) {
			qv = 0
		}
		c := time.Duration(int64(qv))
		if c.Milliseconds() == 0 {
			return FGroup(c.Microseconds()) + "μs"
		} else {
			return FGroup(c.Milliseconds()) + "ms"
		}
	}

	fmt.Fprintf(lw[i], timefmt(pctRoundTripLatency),
		Cyan(convertToMs(p.ReqStats.ElpsdAtmptLatencyNsQuantiles, 0.1)),
		Cyan(convertToMs(p.ReqStats.ElpsdAtmptLatencyNsQuantiles, 0.5)),
		Cyan(convertToMs(p.ReqStats.ElpsdAtmptLatencyNsQuantiles, 0.9)),
		Cyan(convertToMs(p.ReqStats.ElpsdAtmptLatencyNsQuantiles, 0.99)),
	)

	i++
	fmt.Fprintf(lw[i], timefmt(readthroughputMsg),
		Cyan(p.Config.byteCount(int64(p.ReqStats.CurBytesReadPSec))),
		Cyan(perSecondMsg),
		Cyan(p.Config.byteCount(int64(p.ReqStats.MeanBytesReadPSec))),
		Cyan(perSecondMsg),
		Magenta(p.Config.byteCount(int64(p.ReqStats.MaxBytesReadPSec))),
		Magenta(perSecondMsg),
		Cyan(p.Config.byteCount(p.ReqStats.SumBytesRead)))

	i++
	fmt.Fprintf(lw[i], timefmt(writeThroughputMsg),
		Cyan(p.Config.byteCount(int64(p.ReqStats.CurBytesWrittenPSec))),
		Cyan(perSecondMsg),
		Cyan(p.Config.byteCount(int64(p.ReqStats.MeanBytesWrittenPSec))),
		Cyan(perSecondMsg),
		Magenta(p.Config.byteCount(int64(p.ReqStats.MaxBytesWrittenPSec))),
		Magenta(perSecondMsg),
		Cyan(p.Config.byteCount(p.ReqStats.SumBytesWritten)))

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
		if rand.Float64() < p.Config.Exec.LogSampling {
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
	p.OS.PID = os.Getpid()
	if !p.Config.Exec.SkipInetTest {
		go p.getOSINetSpeed(30)
	}
	_, p.OS.LimitOpenFiles = getUlimit()
	p.OS.LimitRAMBytes = getRAMBytes()
	go func() {
	OSStats:
		for {
			select {
			case <-done:
				break OSStats
			default:
				p.doOSOpenConns()
				time.Sleep(time.Millisecond * 100)
			}
		}
	}()
}

func (p *P0d) getOSINetSpeed(maxWaitSeconds int) {
	abort := func(target *OSNet, contextCancel func()) {
		if !(p.OS.isInetTestDone()) {
			if contextCancel != nil {
				contextCancel()
			}
			p.OS.InetTestAborted = true
			p.OS.inetTestError <- struct{}{}
			if target.client != nil {
				target.client.CloseIdleConnections()
			}
		}
	}

	osn, e := NewOSNet()
	if e == nil {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		time.AfterFunc(time.Duration(maxWaitSeconds)*time.Second, func() {
			abort(osn, cancel)
		})

		e2 := osn.Target.DownloadTestContext(ctx, true)
		if e2 == nil {
			p.OS.InetDlSpeedMBits = osn.Target.DLSpeed
			p.OS.inetDlSpeedDoneFlag = true
			p.OS.inetDlSpeedDone <- struct{}{}
		} else {
			abort(osn, cancel)
		}
		e3 := osn.Target.UploadTestContext(ctx, true)
		if e3 == nil {
			p.OS.InetUlSpeedMBits = osn.Target.ULSpeed
			p.OS.inetUlSpeedDoneFlag = true
			p.OS.inetUlSpeedDone <- struct{}{}
		} else {
			abort(osn, cancel)
		}
		e1 := osn.Target.PingTestContext(ctx)
		if e1 == nil {
			p.OS.InetLatencyNs = osn.Target.Latency
			p.OS.inetLatencyDoneFlag = true
			p.OS.inetLatencyDone <- struct{}{}
		} else {
			abort(osn, cancel)
		}
		osn.client.CloseIdleConnections()
	} else {
		abort(osn, nil)
	}
}

func (p *P0d) doOSOpenConns() {
	p.OS.updateLock.Lock()
	oss := NewOSOpenConns(p.OS.PID)
	oss.updateOpenConns(p.Config)
	//we only append this value to the array if the number of open conns has changed since last time.
	if oss.OpenConns != p.getOSOpenConns().OpenConns {
		p.OS.OpenConns = append(p.OS.OpenConns, *oss)
		if oss.OpenConns > p.OS.MaxOpenConns {
			p.OS.MaxOpenConns = oss.OpenConns
		}
	}
	p.OS.updateLock.Unlock()
}

func (p *P0d) getOSOpenConns() OSOpenConns {
	//first time this runs OSS Stats may not have been initialized
	if len(p.OS.OpenConns) == 0 {
		oss := *NewOSOpenConns(os.Getpid())
		oss.OpenConns = 0
		return oss
	} else {
		return p.OS.OpenConns[len(p.OS.OpenConns)-1]
	}
}

func (p *P0d) setTimerPhase(phase TimerPhase) {
	if phase > p.Time.Phase {
		p.Time.Phase = phase
	}
}

func (p *P0d) isTimerPhase(phase TimerPhase) bool {
	return p.Time.Phase == phase
}

func createRunId() string {
	uid, _ := uuid.NewRandom()
	return fmt.Sprintf("p0d-%s-race-%s", Version, uid)
}
