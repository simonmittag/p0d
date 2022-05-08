package p0d

import (
	"github.com/simonmittag/procspy"
	"math"
	"os"
	"time"
)

type ReqStats struct {
	Start                    time.Time
	Elpsd                    time.Duration
	ReqAtmpts                int
	ReqAtmptsPSec            int
	SumBytesRead             int64
	MeanBytesReadSec         int
	SumBytesWritten          int64
	MeanBytesWrittenSec      int
	SumElpsdAtmptLatencyNs   time.Duration
	MeanElpsdAtmptLatencyNs  time.Duration
	SumMatchingResponseCodes int
	PctMatchingResponseCodes float32
	SumErrors                int
	PctErrors                float32
	ErrorTypes               map[string]int
}

func (s *ReqStats) update(atmpt ReqAtmpt, now time.Time, cfg Config) {
	s.ReqAtmpts++
	s.Elpsd = now.Sub(s.Start)
	s.ReqAtmptsPSec = int(math.Floor(float64(s.ReqAtmpts) / s.Elpsd.Seconds()))

	s.SumBytesRead += atmpt.ResBytes
	s.MeanBytesReadSec = int(math.Floor(float64(s.SumBytesRead) / s.Elpsd.Seconds()))

	s.SumBytesWritten += atmpt.ReqBytes
	s.MeanBytesWrittenSec = int(math.Floor(float64(s.SumBytesWritten) / s.Elpsd.Seconds()))
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
	Pid          int
	Now          time.Time
	PidOpenConns int
}

func NewOSStats() *OSStats {
	return &OSStats{
		Pid:          os.Getpid(),
		Now:          time.Now(),
		PidOpenConns: 0,
	}
}

func (oss *OSStats) updateOpenConns() {
	cs, e := procspy.Connections(true)
	if e != nil {
		_ = e
	}
	d := 0
	a := 0
	for c := cs.Next(); c != nil; c = cs.Next() {
		a++
		if c.PID == uint(oss.Pid) {
			d++
		}
	}
	oss.PidOpenConns = d
}
