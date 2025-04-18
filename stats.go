package p0d

import (
	"encoding/json"
	"github.com/axiomhq/variance"
	"github.com/showwin/speedtest-go/speedtest"
	"github.com/simonmittag/procspy"
	"github.com/spenczar/tdigest"
	"math"
	"net/http"
	"sync/atomic"
	"time"
)

type Sample struct {
	Server      string
	HTTPVersion string
	TLSVersion  string
	IPVersion   string
	RemoteAddr  string
}

const emptySampleMsg = "not detected"

func NewSample() Sample {
	return Sample{
		Server:      emptySampleMsg,
		HTTPVersion: emptySampleMsg,
		TLSVersion:  emptySampleMsg,
		IPVersion:   emptySampleMsg,
		RemoteAddr:  emptySampleMsg,
	}
}

type ReqStats struct {
	Start                        time.Time
	ElpsdNs                      time.Duration
	ReqAtmpts                    int64
	CurReqAtmptsPSec             int64
	MeanReqAtmptsPSec            int64
	MaxReqAtmptsPSec             int64
	CurBytesReadPSec             int64
	SumBytesRead                 int64
	MeanBytesReadPSec            int64
	MaxBytesReadPSec             int64
	CurBytesWrittenPSec          int64
	SumBytesWritten              int64
	MeanBytesWrittenPSec         int64
	MaxBytesWrittenPSec          int64
	ElpsdAtmptLatencyNsQuantiles *Quantile
	ElpsdAtmptLatencyNs          *Welford
	SumMatchingResponseCodes     int
	PctMatchingResponseCodes     float32
	Sample                       Sample
	SumErrors                    int
	PctErrors                    float32
	ErrorTypes                   map[string]int
}

type Welford struct {
	s *variance.Stats
}

func NewWelford() *Welford {
	return &Welford{s: variance.New()}
}

func (w *Welford) Add(val float64) {
	w.s.Add(val)
}

func (w *Welford) Mean() float64 {
	return w.s.Mean()
}

func (w *Welford) Stddev() float64 {
	return w.s.StandardDeviation()
}

func (w *Welford) StddevPop() float64 {
	return w.s.StandardDeviationPopulation()
}

func (w *Welford) Var() float64 {
	return w.s.Variance()
}

func (w *Welford) VarPop() float64 {
	return w.s.VariancePopulation()
}

func (w *Welford) Cv() float64 {
	mean := w.s.Mean()
	if mean == 0 || math.IsNaN(mean) || math.IsInf(mean, 0) {
		return 0
	}
	return w.s.StandardDeviation() / mean
}

func (w *Welford) Stderr() float64 {
	n := float64(w.s.NumDataValues())
	if n == 0 {
		return 0
	}
	return w.s.StandardDeviation() / math.Sqrt(n)
}

func (w *Welford) MarshalJSON() ([]byte, error) {
	m := make(map[string]float64)
	m["mean"] = w.Mean()
	m["stddev"] = w.Stddev()
	m["cv"] = w.Cv()
	m["stderr"] = w.Stderr()
	return json.Marshal(m)
}

type Quantile struct {
	t *tdigest.TDigest
}

func NewQuantile() *Quantile {
	return &Quantile{
		t: tdigest.New(),
	}
}

func NewQuantileWithCompression(compression float64) *Quantile {
	return &Quantile{
		t: tdigest.NewWithCompression(compression),
	}
}

func (q *Quantile) Add(val float64, weight int) *Quantile {
	q.t.Add(val, weight)
	return q
}

func (q *Quantile) Quantile(v float64) float64 {
	return q.t.Quantile(v)
}

func (q *Quantile) MarshalJSON() ([]byte, error) {
	m := make(map[string]int64)
	m["min"] = int64(math.Ceil(q.t.Quantile(0)))
	m["p10"] = int64(math.Ceil(q.t.Quantile(0.1)))
	m["p16"] = int64(math.Ceil(q.t.Quantile(0.16)))
	m["p25"] = int64(math.Ceil(q.t.Quantile(0.25)))
	m["p50"] = int64(math.Ceil(q.t.Quantile(0.50)))
	m["p75"] = int64(math.Ceil(q.t.Quantile(0.75)))
	m["p84"] = int64(math.Ceil(q.t.Quantile(0.84)))
	m["p90"] = int64(math.Ceil(q.t.Quantile(0.90)))
	m["p99"] = int64(math.Ceil(q.t.Quantile(0.99)))
	m["max"] = int64(math.Ceil(q.t.Quantile(1)))
	return json.Marshal(m)
}

func (s *ReqStats) update(atmpt ReqAtmpt, now time.Time, cfg Config) {
	s.ReqAtmpts++
	s.ElpsdNs = now.Sub(s.Start)

	s.MeanReqAtmptsPSec = int64(math.Floor(float64(s.ReqAtmpts) / s.ElpsdNs.Seconds()))

	crs := atomic.AddInt64(&s.CurReqAtmptsPSec, 1)
	if crs > s.MaxReqAtmptsPSec {
		s.MaxReqAtmptsPSec = crs
	}
	time.AfterFunc(time.Second*1, func() {
		atomic.AddInt64(&s.CurReqAtmptsPSec, -1)
	})

	crbs := atomic.AddInt64(&s.CurBytesReadPSec, atmpt.ResBytes)
	if crbs > s.MaxBytesReadPSec {
		s.MaxBytesReadPSec = crbs
	}
	time.AfterFunc(time.Second*1, func() {
		atomic.AddInt64(&s.CurBytesReadPSec, -atmpt.ResBytes)
	})

	s.SumBytesRead += atmpt.ResBytes
	s.MeanBytesReadPSec = int64(math.Floor(float64(s.SumBytesRead) / s.ElpsdNs.Seconds()))
	if s.MeanBytesReadPSec > s.MaxBytesReadPSec {
		s.MaxBytesReadPSec = s.MeanBytesReadPSec
	}

	cwbs := atomic.AddInt64(&s.CurBytesWrittenPSec, atmpt.ReqBytes)
	if cwbs > s.MaxBytesWrittenPSec {
		s.MaxBytesWrittenPSec = cwbs
	}
	time.AfterFunc(time.Second*1, func() {
		atomic.AddInt64(&s.CurBytesWrittenPSec, -atmpt.ReqBytes)
	})

	s.SumBytesWritten += atmpt.ReqBytes
	s.MeanBytesWrittenPSec = int64(math.Floor(float64(s.SumBytesWritten) / s.ElpsdNs.Seconds()))
	if s.MeanBytesWrittenPSec > s.MaxBytesWrittenPSec {
		s.MaxBytesWrittenPSec = s.MeanBytesWrittenPSec
	}
	s.ElpsdAtmptLatencyNs.Add(float64(atmpt.ElpsdNs))
	s.ElpsdAtmptLatencyNsQuantiles.Add(float64(atmpt.ElpsdNs.Nanoseconds()), 1)

	if atmpt.ResCode == cfg.Res.Code {
		s.SumMatchingResponseCodes++
	}
	s.PctMatchingResponseCodes = 100 * (float32(s.SumMatchingResponseCodes) / float32(s.ReqAtmpts))

	if atmpt.ResErr != "" {
		s.SumErrors++
		s.ErrorTypes[atmpt.ResErr]++
	}
	s.PctErrors = 100 * (float32(s.SumErrors) / float32(s.ReqAtmpts))
}

type OSOpenConns struct {
	Time      time.Time
	OpenConns int
	PID       int
}

func NewOSOpenConns(pid int) *OSOpenConns {
	return &OSOpenConns{
		Time:      time.Now(),
		OpenConns: 0,
		PID:       pid,
	}
}

func (oss *OSOpenConns) updateOpenConns(cfg Config) {
	cs, e := procspy.Connections(true)
	if e != nil {
		_ = e
	} else {
		d := 0
		for c := cs.Next(); c != nil; c = cs.Next() {
			// fixes bug where PID connections to other network infra are reported as false positive, see:
			// https://github.com/simonmittag/p0d/issues/31
			if c.PID == uint(oss.PID) {
				for _, ip := range cfg.Req.Ips {
					if c.RemotePort == cfg.getRemotePort() &&
						ip.Equal(c.RemoteAddress) {
						d++
					}
				}
			}
		}
		oss.OpenConns = d
	}
}

type OSNet struct {
	Target *speedtest.Server
	client *http.Client
}

func NewOSNet() (*OSNet, error) {
	closeIdler := &http.Client{}
	spdt := speedtest.New(speedtest.WithDoer(closeIdler))
	servers, e2 := spdt.FetchServers()
	if e2 != nil {
		return nil, e2
	}
	closeIdler.CloseIdleConnections()

	targets, e3 := servers.FindServer([]int{})
	if len(targets) == 0 || e3 != nil {
		return nil, e3
	}
	servers = nil

	return &OSNet{
		Target: targets[0],
		client: closeIdler,
	}, nil
}
