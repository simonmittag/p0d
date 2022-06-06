package p0d

import (
	"fmt"
	"github.com/axiomhq/variance"
	"math"
	"testing"
	"time"
)

func TestUpdateStats(t *testing.T) {
	cfg := Config{Res: Res{Code: 200}}

	s := ReqStats{
		ReqAtmpts:                    11,
		Start:                        time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		ErrorTypes:                   make(map[string]int),
		ElpsdAtmptLatencyNsQuantiles: NewQuantile(),
		ElpsdAtmptLatencyNs:          &Welford{s: variance.New()},
	}

	g := ReqAtmpt{
		Start:    time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC),
		Stop:     time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC),
		ElpsdNs:  time.Duration(2 * time.Second),
		ResCode:  200,
		ReqBytes: 4000,
		ResBytes: 1000,
		ResErr:   "",
	}

	now := time.Date(2000, 1, 1, 0, 0, 4, 0, time.UTC)
	s.update(g, now, cfg)

	if s.ReqAtmpts != 12 {
		t.Error("request attempts incorrect")
	}
	if s.MeanReqAtmptsPSec != 3 {
		t.Error("request attempts per second incorrect")
	}

	if s.CurBytesReadPSec != 1000 {
		t.Error("curbytesreadpsec incorrect")
	}
	if s.CurBytesWrittenPSec != 4000 {
		t.Error("curbyteswrittenpsec incorrect")
	}

	time.Sleep(time.Millisecond * 500)
	if s.CurBytesReadPSec != 1000 {
		t.Error("curbytesreadpsec incorrect")
	}
	if s.CurBytesWrittenPSec != 4000 {
		t.Error("curbyteswrittenpsec incorrect")
	}

	time.Sleep(time.Millisecond * 510)
	if s.CurBytesReadPSec != 0 {
		t.Error("curbytesreadpsec incorrect")
	}
	if s.CurBytesWrittenPSec != 0 {
		t.Error("curbyteswrittenpsec incorrect")
	}

	if s.SumBytesRead != 1000 {
		t.Error("sumbytesread incorrect")
	}
	if s.MeanBytesReadPSec != 250 {
		t.Error("mean bytes read per sec incorrect")
	}
	if s.MaxBytesReadPSec != 1000 {
		t.Error("max bytes read per sec incorrect")
	}

	if s.SumBytesWritten != 4000 {
		t.Error("sumbyteswritten incorrect")
	}
	if s.MeanBytesWrittenPSec != 1000 {
		t.Error("mean bytes written per sec incorrect")
	}
	if s.MaxBytesWrittenPSec != 4000 {
		t.Error("max bytes written per sec incorrect")
	}

	if s.ElpsdNs != 4*time.Second {
		t.Error("status elapsed time incorrect")
	}
	//this test result looks off but is right. we only ran update() once
	if time.Duration(s.ElpsdAtmptLatencyNs.Mean()).Milliseconds() != 2000 {
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
	if s.SumBytesRead != 2000 {
		t.Error("sumbytes incorrect")
	}
	if s.MeanBytesReadPSec != 333 {
		t.Error("mean bytes per sec incorrect")
	}
	if s.ElpsdNs != 6*time.Second {
		t.Error("status elapsed time incorrect")
	}
	//this looks off again but is correct. 200
	if time.Duration(s.ElpsdAtmptLatencyNs.Mean()).Milliseconds() != 1500 {
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

func TestUpdateOSStats(t *testing.T) {
	oss := NewOSOpenConns(1)
	oss.updateOpenConns(Config{Exec: Exec{Concurrency: 3}})
}

func TestOnlineVariance(t *testing.T) {
	floatThresh := 0.00000001
	vals := []float64{56, 65, 74, 75, 76, 77, 80, 81, 91}
	w := NewWelford()
	for _, v := range vals {
		w.Add(v)
	}
	wantVariancePop := float64(784. / 9.)
	if wantVariancePop-w.VarPop() > floatThresh {
		t.Errorf("incorrect pop variance, want %f, got %f", wantVariancePop, w.VarPop())
	}
	wantStddevPop := math.Sqrt(wantVariancePop)
	if wantStddevPop-w.StddevPop() > floatThresh {
		t.Errorf("incorrect pop stddev, want %f, got %f", wantStddevPop, w.StddevPop())
	}
	wantVariance := float64(784. / 8.)
	if wantVariance-w.Var() > floatThresh {
		t.Errorf("incorrect variance, want %f, got %f", wantVariance, w.Var())
	}
	wantStddev := math.Sqrt(wantVariance)
	if wantStddev-w.Stddev() > floatThresh {
		t.Errorf("incorrect stddev, want %f, got %f", wantStddev, w.Stddev())
	}
	t.Logf("stderr: %v", w.Stderr())

}

func BenchmarkUpdateOpenConns(b *testing.B) {
	for n := 0; n < b.N; n++ {
		oss := NewOSOpenConns(1)
		oss.updateOpenConns(Config{})
	}
}

func BenchmarkNewOSNet(b *testing.B) {
	for n := 0; n < b.N; n++ {
		oss, e := NewOSNet()
		_ = oss
		_ = e
	}
}
