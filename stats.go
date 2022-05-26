package p0d

import (
	"github.com/simonmittag/procspy"
	"math"
	"time"
)

type Sample struct {
	HTTPVersion string
	TLSVersion  string
	IPVersion   string
	RemoteAddr  string
}

const emptySampleMsg = "not detected"

func NewSample() Sample {
	return Sample{
		HTTPVersion: emptySampleMsg,
		TLSVersion:  emptySampleMsg,
		IPVersion:   emptySampleMsg,
		RemoteAddr:  emptySampleMsg,
	}
}

type ReqStats struct {
	Start                    time.Time
	Elpsd                    time.Duration
	ReqAtmpts                int
	ReqAtmptsPSec            int
	MaxReqAtmptsPSec         int
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
	s.ReqAtmptsPSec = int(math.Floor(float64(s.ReqAtmpts) / s.Elpsd.Seconds()))
	if s.ReqAtmptsPSec > s.MaxReqAtmptsPSec {
		s.MaxReqAtmptsPSec = s.ReqAtmptsPSec
	}

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

type OSStats struct {
	Pid       int
	Now       time.Time
	OpenConns int
}

func NewOSStats(pid int) *OSStats {
	return &OSStats{
		Pid:       pid,
		Now:       time.Now(),
		OpenConns: 0,
	}
}

func (oss *OSStats) updateOpenConns(cfg Config) {
	//TODO: this produces a 0 when it shouldn't. after signalling CTRL+C this often returns intermittent 0
	cs, e := procspy.Connections(true)
	if e != nil {
		_ = e
	} else {
		d := 0
		for c := cs.Next(); c != nil; c = cs.Next() {
			// fixes bug where pid connections to other network infra are reported as false positive, see:
			// https://github.com/simonmittag/p0d/issues/31
			if c.PID == uint(oss.Pid) && c.RemotePort == cfg.getRemotePort() {
				d++
			}
		}
		oss.OpenConns = d
	}
}
