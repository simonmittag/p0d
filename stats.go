package p0d

import (
	"github.com/simonmittag/procspy"
	"math"
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
	Elpsd                    time.Duration
	ReqAtmpts                int64
	CurReqAtmptsPSec         int64
	MeanReqAtmptsPSec        int64
	MaxReqAtmptsPSec         int64
	SumBytesRead             int64
	MeanBytesReadSec         int
	MaxBytesReadSec          int
	SumBytesWritten          int64
	MeanBytesWrittenSec      int
	MaxBytesWrittenSec       int
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
	s.Elpsd = now.Sub(s.Start)
	s.MeanReqAtmptsPSec = int64(math.Floor(float64(s.ReqAtmpts) / s.Elpsd.Seconds()))

	crs := atomic.AddInt64(&s.CurReqAtmptsPSec, 1)
	if crs > s.MaxReqAtmptsPSec {
		s.MaxReqAtmptsPSec = crs
	}
	time.AfterFunc(time.Second*1, func() {
		atomic.AddInt64(&s.CurReqAtmptsPSec, -1)
	})

	s.SumBytesRead += atmpt.ResBytes
	s.MeanBytesReadSec = int(math.Floor(float64(s.SumBytesRead) / s.Elpsd.Seconds()))
	if s.MeanBytesReadSec > s.MaxBytesReadSec {
		s.MaxBytesReadSec = s.MeanBytesReadSec
	}

	s.SumBytesWritten += atmpt.ReqBytes
	s.MeanBytesWrittenSec = int(math.Floor(float64(s.SumBytesWritten) / s.Elpsd.Seconds()))
	if s.MeanBytesWrittenSec > s.MaxBytesWrittenSec {
		s.MaxBytesWrittenSec = s.MeanBytesWrittenSec
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
	Now       time.Time
	OpenConns int
	pid       int
}

func NewOSStats(pid int) *OSOpenConns {
	return &OSOpenConns{
		Now:       time.Now(),
		OpenConns: 0,
		pid:       pid,
	}
}

func (oss *OSOpenConns) updateOpenConns(cfg Config) {
	//TODO: this produces a 0 when it shouldn't. after signalling CTRL+C this often returns intermittent 0
	cs, e := procspy.Connections(true)
	if e != nil {
		_ = e
	} else {
		d := 0
		for c := cs.Next(); c != nil; c = cs.Next() {
			// fixes bug where pid connections to other network infra are reported as false positive, see:
			// https://github.com/simonmittag/p0d/issues/31
			if c.PID == uint(oss.pid) && c.RemotePort == cfg.getRemotePort() {
				d++
			}
		}
		oss.OpenConns = d
	}
}
