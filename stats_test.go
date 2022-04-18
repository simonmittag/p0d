package p0d

import (
	"fmt"
	"testing"
	"time"
)

func TestUpdateStats(t *testing.T) {
	cfg := Config{Res: Res{Code: 200}}

	s := Stats{
		ReqAtmpts:  11,
		Start:      time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		ErrorTypes: make(map[string]int),
	}

	g := ReqAtmpt{
		Start:    time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC),
		Stop:     time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC),
		ElpsdNs:  time.Duration(2 * time.Second),
		ResCode:  200,
		ResBytes: 1000,
		ResErr:   "",
	}

	now := time.Date(2000, 1, 1, 0, 0, 4, 0, time.UTC)
	s.update(g, now, cfg)

	if s.ReqAtmpts != 12 {
		t.Error("request attempts incorrect")
	}
	if s.ReqAtmptsSec != 3 {
		t.Error("request attempts per second incorrect")
	}
	if s.SumBytes != 1000 {
		t.Error("sumbytes incorrect")
	}
	if s.MeanBytesSec != 250 {
		t.Error("mean bytes per sec incorrect")
	}
	if s.Elpsd != 4*time.Second {
		t.Error("status elapsed time incorrect")
	}
	if s.SumElpsdAtmptLatency.Milliseconds() != 2000 {
		t.Error("sum of elapsed time for attempts incorrect")
	}
	if s.MeanElpsdAtmptLatency.Milliseconds() != 166 {
		t.Error("mean of elapsed time for attempts incorrect")
	}
	if s.SumMatchingResponseCodes != 1 {
		t.Error("sum matching response codes incorrect")
	}
	if fmt.Sprintf("%.2f", s.PctMatchingResponseCodes) != "8.33" {
		t.Error("percent matching response codes incorrect")
	}
	if s.SumErrors > 0 || s.PctErrors > 0 {
		t.Error("should not have errors")
	}

	now = time.Date(2000, 1, 1, 0, 0, 6, 0, time.UTC)

	//we update the stats now with more data, this time 1s req but it's 6s down the timeline
	//this request has an error
	g2 := ReqAtmpt{
		Start:    time.Date(2000, 1, 1, 0, 0, 5, 0, time.UTC),
		Stop:     time.Date(2000, 1, 1, 0, 0, 6, 0, time.UTC),
		ElpsdNs:  time.Duration(1 * time.Second),
		ResCode:  201,
		ResBytes: 1000,
		ResErr:   "i'm so bad",
	}

	s.update(g2, now, cfg)

	if s.ReqAtmpts != 13 {
		t.Error("request attempts incorrect")
	}
	if s.SumBytes != 2000 {
		t.Error("sumbytes incorrect")
	}
	if s.MeanBytesSec != 333 {
		t.Error("mean bytes per sec incorrect")
	}
	if s.Elpsd != 6*time.Second {
		t.Error("status elapsed time incorrect")
	}
	if s.SumElpsdAtmptLatency != 3*time.Second {
		t.Error("sum of elapsed time for attempts incorrect")
	}
	if s.MeanElpsdAtmptLatency.Milliseconds() != 230 {
		t.Error("mean of elapsed time for attempts incorrect")
	}
	if s.SumMatchingResponseCodes != 1 {
		t.Error("sum matching response codes incorrect")
	}
	if fmt.Sprintf("%.2f", s.PctMatchingResponseCodes) != "7.69" {
		t.Error("percent matching response codes incorrect")
	}
	if s.SumErrors != 1 {
		t.Error("should have errors")
	}
	if fmt.Sprintf("%.2f", s.PctErrors) != "7.69" {
		t.Error("should have errors")
	}
}
