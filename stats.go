package p0d

import (
	"math"
	"time"
)

type Stats struct {
	Start                    time.Time
	Elpsd                    time.Duration
	ReqAtmpts                int
	ReqAtmptsPSec            int
	SumBytesRead             int64
	MeanBytesReadSec         int
	SumBytesWritten          int64
	MeanBytesWrittenSec      int
	SumElpsdAtmptLatency     time.Duration
	MeanElpsdAtmptLatency    time.Duration
	SumMatchingResponseCodes int
	PctMatchingResponseCodes float32
	SumErrors                int
	PctErrors                float32
	ErrorTypes               map[string]int
}

func (s *Stats) update(atmpt ReqAtmpt, now time.Time, cfg Config) {
	s.ReqAtmpts++
	s.Elpsd = now.Sub(s.Start)
	s.ReqAtmptsPSec = int(math.Floor(float64(s.ReqAtmpts) / s.Elpsd.Seconds()))

	s.SumBytesRead += atmpt.ResBytes
	s.MeanBytesReadSec = int(math.Floor(float64(s.SumBytesRead) / s.Elpsd.Seconds()))

	s.SumBytesWritten += atmpt.ReqBytes
	s.MeanBytesWrittenSec = int(math.Floor(float64(s.SumBytesWritten) / s.Elpsd.Seconds()))
	s.SumElpsdAtmptLatency += atmpt.ElpsdNs
	s.MeanElpsdAtmptLatency = s.SumElpsdAtmptLatency / time.Duration(s.ReqAtmpts)

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
