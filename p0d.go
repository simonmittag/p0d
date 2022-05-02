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
			p.finaliseOutputAndCloseWriters()
			p.Interrupted = true
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

	log("done")
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
	fmt.Printf("%v", "\n\u001B[38;2;14;36;45m.\u001B[0m\u001B[38;2;46;114;144m-\u001B[0m\u001B[38;2;61;150;189m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;65;159;201m+\u001B[0m\u001B[38;2;62;153;193m+\u001B[0m\u001B[38;2;60;149;188m=\u001B[0m\u001B[38;2;64;157;198m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;66;162;205m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;52;119;150m=\u001B[0m\u001B[38;2;28;60;75m:\u001B[0m\u001B[38;2;19;20;25m \u001B[0m\u001B[38;2;34;9;9m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;9;2;2m \u001B[0m\u001B[38;2;200;52;51m-\u001B[0m\u001B[38;2;231;60;59m=\u001B[0m\u001B[38;2;41;10;10m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;64;16;16m.\u001B[0m\u001B[38;2;193;50;49m-\u001B[0m\u001B[38;2;200;52;51m-\u001B[0m\u001B[38;2;10;2;2m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;12;23;28m \u001B[0m\u001B[38;2;39;91;115m-\u001B[0m\u001B[38;2;64;155;196m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;65;159;201m+\u001B[0m\u001B[38;2;65;160;202m+\u001B[0m\u001B[38;2;64;157;198m+\u001B[0m\u001B[38;2;65;160;202m+\u001B[0m\u001B[38;2;65;161;203m+\u001B[0m\u001B[38;2;66;162;204m+\u001B[0m\u001B[38;2;62;151;191m+\u001B[0m\u001B[38;2;51;125;158m=\u001B[0m\u001B[38;2;33;81;103m:\u001B[0m\u001B[38;2;5;13;17m \u001B[0m\n\u001B[38;2;61;151;190m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;42;103;130m-\u001B[0m\u001B[38;2;10;25;32m \u001B[0m\u001B[38;2;7;18;22m \u001B[0m\u001B[38;2;9;23;29m \u001B[0m\u001B[38;2;60;147;186m=\u001B[0m\u001B[38;2;66;163;206m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;56;138;175m=\u001B[0m\u001B[38;2;16;44;56m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;26;4;3m \u001B[0m\u001B[38;2;209;54;53m-\u001B[0m\u001B[38;2;197;51;50m-\u001B[0m\u001B[38;2;97;25;24m.\u001B[0m\u001B[38;2;7;2;1m \u001B[0m\u001B[38;2;138;36;35m:\u001B[0m\u001B[38;2;245;64;63m=\u001B[0m\u001B[38;2;77;20;20m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;242;63;62m=\u001B[0m\u001B[38;2;201;52;51m-\u001B[0m\u001B[38;2;55;14;14m.\u001B[0m\u001B[38;2;55;14;14m.\u001B[0m\u001B[38;2;134;35;34m:\u001B[0m\u001B[38;2;169;44;43m-\u001B[0m\u001B[38;2;151;39;39m:\u001B[0m\u001B[38;2;51;11;10m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;3;25;32m \u001B[0m\u001B[38;2;46;114;144m-\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;60;148;187m=\u001B[0m\u001B[38;2;15;38;48m.\u001B[0m\u001B[38;2;14;36;46m.\u001B[0m\u001B[38;2;19;47;60m.\u001B[0m\u001B[38;2;50;124;157m=\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;54;133;168m=\u001B[0m\n\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;65;159;201m+\u001B[0m\u001B[38;2;65;160;202m+\u001B[0m\u001B[38;2;22;54;68m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;17;43;55m.\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;66;163;206m+\u001B[0m\u001B[38;2;60;145;183m=\u001B[0m\u001B[38;2;5;14;17m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;120;32;32m:\u001B[0m\u001B[38;2;120;33;33m:\u001B[0m\u001B[38;2;82;22;21m.\u001B[0m\u001B[38;2;80;21;20m.\u001B[0m\u001B[38;2;128;33;33m:\u001B[0m\u001B[38;2;174;45;44m-\u001B[0m\u001B[38;2;148;38;38m:\u001B[0m\u001B[38;2;38;10;9m \u001B[0m\u001B[38;2;130;34;33m:\u001B[0m\u001B[38;2;122;32;31m:\u001B[0m\u001B[38;2;103;27;26m.\u001B[0m\u001B[38;2;154;40;39m:\u001B[0m\u001B[38;2;37;9;9m \u001B[0m\u001B[38;2;115;30;29m:\u001B[0m\u001B[38;2;163;42;42m-\u001B[0m\u001B[38;2;144;37;37m:\u001B[0m\u001B[38;2;113;29;29m:\u001B[0m\u001B[38;2;90;23;23m.\u001B[0m\u001B[38;2;80;21;21m.\u001B[0m\u001B[38;2;74;20;20m.\u001B[0m\u001B[38;2;33;5;4m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;39;97;122m-\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;14;36;45m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;26;63;80m:\u001B[0m\u001B[38;2;66;162;205m+\u001B[0m\u001B[38;2;65;159;201m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\n\u001B[38;2;43;105;133m-\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;61;150;189m+\u001B[0m\u001B[38;2;42;102;129m-\u001B[0m\u001B[38;2;40;99;125m-\u001B[0m\u001B[38;2;54;133;168m=\u001B[0m\u001B[38;2;66;162;204m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;34;83;105m:\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;1;3;4m \u001B[0m\u001B[38;2;59;15;15m.\u001B[0m\u001B[38;2;119;31;30m:\u001B[0m\u001B[38;2;145;37;37m:\u001B[0m\u001B[38;2;141;36;36m:\u001B[0m\u001B[38;2;117;30;30m:\u001B[0m\u001B[38;2;98;25;25m.\u001B[0m\u001B[38;2;100;26;25m.\u001B[0m\u001B[38;2;116;30;29m:\u001B[0m\u001B[38;2;171;44;43m-\u001B[0m\u001B[38;2;93;24;23m.\u001B[0m\u001B[38;2;76;20;19m.\u001B[0m\u001B[38;2;147;38;38m:\u001B[0m\u001B[38;2;171;44;44m-\u001B[0m\u001B[38;2;94;24;24m.\u001B[0m\u001B[38;2;103;27;26m.\u001B[0m\u001B[38;2;128;33;33m:\u001B[0m\u001B[38;2;154;40;39m:\u001B[0m\u001B[38;2;167;43;42m-\u001B[0m\u001B[38;2;158;41;40m:\u001B[0m\u001B[38;2;131;34;33m:\u001B[0m\u001B[38;2;80;22;22m.\u001B[0m\u001B[38;2;1;1;1m \u001B[0m\u001B[38;2;3;7;9m \u001B[0m\u001B[38;2;61;150;189m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;31;75;95m:\u001B[0m\u001B[38;2;0;1;1m \u001B[0m\u001B[38;2;0;1;2m \u001B[0m\u001B[38;2;27;67;85m:\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;66;162;204m+\u001B[0m\u001B[38;2;65;161;203m+\u001B[0m\n\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;14;36;46m.\u001B[0m\u001B[38;2;30;73;93m:\u001B[0m\u001B[38;2;40;99;125m-\u001B[0m\u001B[38;2;47;115;145m-\u001B[0m\u001B[38;2;46;114;144m-\u001B[0m\u001B[38;2;55;136;172m=\u001B[0m\u001B[38;2;65;161;203m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;42;100;127m-\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;26;10;11m \u001B[0m\u001B[38;2;110;29;29m:\u001B[0m\u001B[38;2;135;35;34m:\u001B[0m\u001B[38;2;145;37;37m:\u001B[0m\u001B[38;2;137;35;35m:\u001B[0m\u001B[38;2;118;30;30m:\u001B[0m\u001B[38;2;99;26;25m.\u001B[0m\u001B[38;2;94;24;24m.\u001B[0m\u001B[38;2;118;30;30m:\u001B[0m\u001B[38;2;154;40;39m:\u001B[0m\u001B[38;2;96;25;24m.\u001B[0m\u001B[38;2;94;24;24m.\u001B[0m\u001B[38;2;156;40;40m:\u001B[0m\u001B[38;2;108;28;27m:\u001B[0m\u001B[38;2;122;31;31m:\u001B[0m\u001B[38;2;119;31;30m:\u001B[0m\u001B[38;2;102;26;26m.\u001B[0m\u001B[38;2;116;30;29m:\u001B[0m\u001B[38;2;147;38;38m:\u001B[0m\u001B[38;2;170;44;43m-\u001B[0m\u001B[38;2;153;40;39m:\u001B[0m\u001B[38;2;63;19;19m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;9;18;23m \u001B[0m\u001B[38;2;62;153;194m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;31;77;97m:\u001B[0m\u001B[38;2;0;1;1m \u001B[0m\u001B[38;2;0;1;2m \u001B[0m\u001B[38;2;27;66;84m:\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;66;162;204m+\u001B[0m\u001B[38;2;66;162;204m+\u001B[0m\n\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;15;37;47m.\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;65;161;203m+\u001B[0m\u001B[38;2;65;159;201m+\u001B[0m\u001B[38;2;20;48;61m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;44;8;6m \u001B[0m\u001B[38;2;70;18;17m.\u001B[0m\u001B[38;2;78;20;20m.\u001B[0m\u001B[38;2;100;26;25m.\u001B[0m\u001B[38;2;135;35;34m:\u001B[0m\u001B[38;2;171;44;43m-\u001B[0m\u001B[38;2;162;42;41m-\u001B[0m\u001B[38;2;70;18;18m.\u001B[0m\u001B[38;2;58;15;15m.\u001B[0m\u001B[38;2;209;54;53m-\u001B[0m\u001B[38;2;32;8;8m \u001B[0m\u001B[38;2;167;43;42m-\u001B[0m\u001B[38;2;107;28;27m:\u001B[0m\u001B[38;2;44;11;11m \u001B[0m\u001B[38;2;149;39;38m:\u001B[0m\u001B[38;2;191;49;49m-\u001B[0m\u001B[38;2;154;40;39m:\u001B[0m\u001B[38;2;77;20;20m.\u001B[0m\u001B[38;2;33;8;8m \u001B[0m\u001B[38;2;56;14;14m.\u001B[0m\u001B[38;2;31;2;0m \u001B[0m\u001B[38;2;2;6;8m \u001B[0m\u001B[38;2;49;119;150m=\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;16;39;50m.\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;25;63;79m:\u001B[0m\u001B[38;2;66;163;206m+\u001B[0m\u001B[38;2;65;159;201m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\n\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;1;2m \u001B[0m\u001B[38;2;1;2;3m \u001B[0m\u001B[38;2;1;2;3m \u001B[0m\u001B[38;2;1;2;3m \u001B[0m\u001B[38;2;1;3;4m \u001B[0m\u001B[38;2;2;5;6m \u001B[0m\u001B[38;2;55;135;170m=\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;37;95;120m-\u001B[0m\u001B[38;2;1;24;32m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;79;19;19m.\u001B[0m\u001B[38;2;158;41;40m:\u001B[0m\u001B[38;2;153;40;39m:\u001B[0m\u001B[38;2;97;25;24m.\u001B[0m\u001B[38;2;22;5;5m \u001B[0m\u001B[38;2;107;28;27m:\u001B[0m\u001B[38;2;245;64;63m=\u001B[0m\u001B[38;2;181;47;46m-\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;136;35;35m:\u001B[0m\u001B[38;2;245;64;63m=\u001B[0m\u001B[38;2;137;35;35m:\u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;66;17;17m.\u001B[0m\u001B[38;2;163;42;42m-\u001B[0m\u001B[38;2;181;47;46m-\u001B[0m\u001B[38;2;8;1;1m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;12;46;60m.\u001B[0m\u001B[38;2;56;138;174m=\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;62;153;194m+\u001B[0m\u001B[38;2;17;42;53m.\u001B[0m\u001B[38;2;12;31;39m.\u001B[0m\u001B[38;2;14;36;46m.\u001B[0m\u001B[38;2;46;114;144m-\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;60;148;187m=\u001B[0m\n\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;1;2m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;17;41;52m.\u001B[0m\u001B[38;2;57;141;178m=\u001B[0m\u001B[38;2;57;139;176m=\u001B[0m\u001B[38;2;56;138;174m=\u001B[0m\u001B[38;2;59;145;183m=\u001B[0m\u001B[38;2;52;123;155m=\u001B[0m\u001B[38;2;32;73;92m:\u001B[0m\u001B[38;2;6;19;24m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;34;9;8m \u001B[0m\u001B[38;2;191;49;49m-\u001B[0m\u001B[38;2;133;34;34m:\u001B[0m\u001B[38;2;23;6;5m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;38;10;9m \u001B[0m\u001B[38;2;189;49;48m-\u001B[0m\u001B[38;2;130;34;33m:\u001B[0m\u001B[38;2;1;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;0;0;0m \u001B[0m\u001B[38;2;4;11;14m \u001B[0m\u001B[38;2;36;84;106m:\u001B[0m\u001B[38;2;60;145;183m=\u001B[0m\u001B[38;2;62;152;192m+\u001B[0m\u001B[38;2;60;149;188m=\u001B[0m\u001B[38;2;63;154;195m+\u001B[0m\u001B[38;2;64;156;198m+\u001B[0m\u001B[38;2;66;163;206m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;67;164;207m+\u001B[0m\u001B[38;2;59;144;182m=\u001B[0m\u001B[38;2;43;106;134m-\u001B[0m\u001B[38;2;12;30;38m.\u001B[0m\n\n")

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
	log("%s starting engines", Yellow(p.ID))
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
	fmt.Fprintf(lw[2], timefmt("roundtrip throughput: %s%s"), Cyan(FGroup(int64(p.Stats.ReqAtmptsSec))), Cyan("/s"))
	fmt.Fprintf(lw[3], timefmt("roundtrip latency: %s%s"), Cyan(FGroup(int64(p.Stats.MeanElpsdAtmptLatency.Milliseconds()))), Cyan("ms"))
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
