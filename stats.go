package p0d

import (
	"github.com/showwin/speedtest-go/speedtest"
	"github.com/simonmittag/procspy"
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
	Start                    time.Time
	ElpsdNs                  time.Duration
	ReqAtmpts                int64
	CurReqAtmptsPSec         int64
	MeanReqAtmptsPSec        int64
	MaxReqAtmptsPSec         int64
	SumBytesRead             int64
	MeanBytesReadPSec        int
	MaxBytesReadPSec         int
	SumBytesWritten          int64
	MeanBytesWrittenPSec     int
	MaxBytesWrittenPSec      int
	SumElpsdAtmptLatencyNs   time.Duration
	MeanElpsdAtmptLatencyNs  time.Duration
	SumMatchingResponseCodes int
	PctMatchingResponseCodes float32
	Sample                   Sample
	SumErrors                int
	PctErrors                float32
	ErrorTypes               map[string]int
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

	s.SumBytesRead += atmpt.ResBytes
	s.MeanBytesReadPSec = int(math.Floor(float64(s.SumBytesRead) / s.ElpsdNs.Seconds()))
	if s.MeanBytesReadPSec > s.MaxBytesReadPSec {
		s.MaxBytesReadPSec = s.MeanBytesReadPSec
	}

	s.SumBytesWritten += atmpt.ReqBytes
	s.MeanBytesWrittenPSec = int(math.Floor(float64(s.SumBytesWritten) / s.ElpsdNs.Seconds()))
	if s.MeanBytesWrittenPSec > s.MaxBytesWrittenPSec {
		s.MaxBytesWrittenPSec = s.MeanBytesWrittenPSec
	}
	s.SumElpsdAtmptLatencyNs += atmpt.ElpsdNs
	s.MeanElpsdAtmptLatencyNs = s.SumElpsdAtmptLatencyNs / time.Duration(s.ReqAtmpts)

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
			if c.PID == uint(oss.PID) && c.RemotePort == cfg.getRemotePort() {
				d++
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
	user, e1 := spdt.FetchUserInfo()
	if e1 != nil {
		return nil, e1
	}
	servers, e2 := spdt.FetchServers(user)
	if e2 != nil {
		return nil, e2
	}
	closeIdler.CloseIdleConnections()

	targets, e3 := servers.FindServer([]int{})
	if len(targets) == 0 || e3 != nil {
		return nil, e3
	}
	user = nil
	servers = nil

	return &OSNet{
		Target: targets[0],
		client: closeIdler,
	}, nil
}
