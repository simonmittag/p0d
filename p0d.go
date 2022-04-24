package p0d

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gosuri/uilive"
	"github.com/hako/durafmt"
	. "github.com/logrusorgru/aurora"
	"math"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"time"
)

const Version string = "v0.2.3"

type P0d struct {
	ID             string
	Config         Config
	client         *http.Client
	Stats          *Stats
	Start          time.Time
	Stop           time.Time
	Output         string
	OsMaxOpenFiles int64
}

type ReqAtmpt struct {
	Start    time.Time
	Stop     time.Time
	ElpsdNs  time.Duration
	ResCode  int
	ResBytes int64
	ResErr   string
}

func createRunId() string {
	uid, _ := uuid.NewRandom()
	return fmt.Sprintf("p0d-%s-race-%s", Version, uid)
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
	}
}

func NewP0dFromFile(f string, o string) *P0d {
	cfg := loadConfigFromFile(f)
	cfg = cfg.validate()

	start := time.Now()
	_, ul := getUlimit()
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
	}
}

func (p *P0d) Race() {
	_, p.OsMaxOpenFiles = getUlimit()
	p.logBootstrap()

	end := make(chan struct{})
	ras := make(chan ReqAtmpt, 65535)

	time.AfterFunc(time.Duration(p.Config.Exec.DurationSeconds)*time.Second, func() {
		end <- struct{}{}
	})

	checkWrite := func(e error) {
		if e != nil {
			logv(Red("unable to write to output file %s, exiting..."), p.Output)
			os.Exit(-1)
		}
	}

	var aopen = []byte("[")
	var aclose = []byte("]")
	const prefix string = ""
	const indent string = "  "
	var comma = []byte(",\n")

	var ow *os.File
	defer func() {
		if ow != nil {
			ow.Close()
		}
	}()
	var oe error
	if len(p.Output) > 0 {
		ow, oe = os.Create(p.Output)
		checkWrite(oe)
		_, we := ow.Write(aopen)
		checkWrite(we)
	}

	for i := 0; i < p.Config.Exec.Threads; i++ {
		go p.doReqAtmpt(ras)
	}

	l1 := uilive.New()
	//this prevents the writer from flushing inbetween lines. we flush manually after each iteration
	l1.RefreshInterval = time.Hour * 24 * 30
	l1.Start()

	go func() {
		for {
			livelog(l1, p)
			time.Sleep(time.Millisecond * 100)
		}
	}()

Main:
	for {
		select {
		case <-end:
			livelog(l1, p)
			l1.Stop()

			p.Stop = time.Now()
			p.logSummary()

			if len(p.Output) > 0 {
				j, je := json.MarshalIndent(p, prefix, indent)
				checkWrite(je)
				_, we := ow.Write(j)
				checkWrite(we)
				_, we = ow.Write(aclose)
				checkWrite(we)
			}
			break Main
		case ra := <-ras:
			now := time.Now()
			p.Stats.update(ra, now, p.Config)

			if len(p.Output) > 0 {
				rand.Seed(time.Now().UnixNano())
				//only sample a subset of requests
				if rand.Float32() < p.Config.Exec.LogSampling {
					j, je := json.MarshalIndent(ra, prefix, indent)
					checkWrite(je)
					_, we := ow.Write(j)
					checkWrite(we)
					_, we = ow.Write(comma)
					checkWrite(we)
				}
			}
		}
	}
}

func livelog(l1 *uilive.Writer, p *P0d) {
	fmt.Fprintf(l1, timefmt("total HTTP req: %s"), Cyan(FGroup(int64(p.Stats.ReqAtmpts))))
	fmt.Fprintf(l1.Newline(), timefmt("HTTP req throughput: %s%s"), Cyan(FGroup(int64(p.Stats.ReqAtmptsSec))), Cyan("/s"))
	fmt.Fprintf(l1.Newline(), timefmt("req latency: %s%s"), Cyan(FGroup(int64(p.Stats.MeanElpsdAtmptLatency.Milliseconds()))), Cyan("ms"))
	fmt.Fprintf(l1.Newline(), timefmt("bytes read: %s"), Cyan(p.Config.byteCount(p.Stats.SumBytes)))
	fmt.Fprintf(l1.Newline(), timefmt("read throughput: %s%s"), Cyan(p.Config.byteCount(int64(p.Stats.MeanBytesSec))), Cyan("/s"))

	mrc := Cyan(fmt.Sprintf("%s/%s (%s%%)",
		FGroup(int64(p.Stats.SumMatchingResponseCodes)),
		FGroup(int64(p.Stats.ReqAtmpts)),
		fmt.Sprintf("%.2f", math.Floor(float64(p.Stats.PctMatchingResponseCodes*100))/100)))
	fmt.Fprintf(l1.Newline(), timefmt("matching HTTP response codes: %v"), mrc)

	tte := fmt.Sprintf("%s/%s (%s%%)",
		FGroup(int64(p.Stats.SumErrors)),
		FGroup(int64(p.Stats.ReqAtmpts)),
		fmt.Sprintf("%.2f", math.Ceil(float64(p.Stats.PctErrors*100))/100))
	if p.Stats.SumErrors > 0 {
		fmt.Fprintf(l1.Newline(), timefmt("transport errors: %v"), Red(tte))
	} else {
		fmt.Fprintf(l1.Newline(), timefmt("transport errors: %v"), Cyan(tte))
	}
	l1.Flush()
}

func (p *P0d) logBootstrap() {
	if p.OsMaxOpenFiles == 0 {
		msg := Red(fmt.Sprintf("unable to determine OS open file limits"))
		log("%v", msg)
	} else if p.OsMaxOpenFiles <= int64(p.Config.Exec.Connections) {
		msg := fmt.Sprintf("found OS max open file limit %s too low, reduce connections from %s",
			Red(FGroup(int64(p.OsMaxOpenFiles))),
			Red(FGroup(int64(p.Config.Exec.Connections))))
		log(msg)
	} else {
		ul, _ := getUlimit()
		log("found OS open file limits (ulimit): %s", ul)
	}
	log("%s starting...", p.ID)
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
	fmt.Printf(timefmt("=> %s %s"), Yellow(p.Config.Req.Method), Yellow(p.Config.Req.Url))
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

	//truncate runtime as seconds
	elapsed := durafmt.Parse(p.Stop.Sub(p.Start).Truncate(time.Second)).LimitFirstN(2).String()
	log("total runtime: %s", Cyan(elapsed))

	if p.Stats.SumErrors != 0 {
		os.Exit(-1)
	}
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
