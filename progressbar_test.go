package p0d

import (
	"github.com/acarl005/stripansi"
	"strings"
	"testing"
	"time"
)

func TestProgressBarMarkRamp(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	p := P0d{
		Config: Config{
			Exec: Exec{
				DurationSeconds: 30},
		},
	}
	p.Time.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    30,
		size:       20,
		chunkProps: make([]ChunkProps, 20),
	}

	pn := p.Time.Start.Add(3 * time.Second)
	pb.markRamp(pn, &p)

	if pb.chunkProps[pb.chunkPropIndexFor(pn, &p)].isRamp == false {
		t.Error("ramp not set")
	}
}

func TestProgressBarMarkError(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	p := P0d{
		Config: Config{
			Exec: Exec{
				DurationSeconds: 30},
		},
	}
	p.Time.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    30,
		size:       20,
		chunkProps: make([]ChunkProps, 20),
	}

	pn := p.Time.Start.Add(3 * time.Second)
	pb.markError(pn, &p)

	if pb.chunkProps[pb.chunkPropIndexFor(pn, &p)].hasErrors == false {
		t.Error("error not set")
	}
}

func TestProgressBarChunkIndex(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	p := P0d{
		Config: Config{
			Exec: Exec{
				DurationSeconds: 300,
				RampSeconds:     60,
			},
		},
	}
	p.Time.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    300,
		size:       30,
		chunkProps: make([]ChunkProps, 30),
	}

	type chunkTest struct {
		deltaSecond float64
		wantIndex   int
	}

	tests := []chunkTest{
		{deltaSecond: 0.0, wantIndex: 0},
		{deltaSecond: 0.01, wantIndex: 0},
		{deltaSecond: 1.0, wantIndex: 0},
		{deltaSecond: 9.9, wantIndex: 0},
		{deltaSecond: 10, wantIndex: 1},
		{deltaSecond: 59.9, wantIndex: 5},
		{deltaSecond: 60, wantIndex: 6},
		{deltaSecond: 240, wantIndex: 24},
		{deltaSecond: 290, wantIndex: 29},
		{deltaSecond: 300, wantIndex: 29},
	}

	for _, tc := range tests {
		pns := p.Time.Start.Add(time.Duration(tc.deltaSecond * float64(time.Second)))
		got := pb.chunkPropIndexFor(pns, &p)
		if got != tc.wantIndex {
			t.Errorf("bad chunk index for deltaSeconds %v, want %v got %v", tc.deltaSecond, tc.wantIndex, got)
		}
	}
}

func TestProgressBarRenderLength(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	want := 30
	p := P0d{
		Config: Config{
			Exec: Exec{
				DurationSeconds: 300,
				RampSeconds:     60,
			},
		},
	}
	p.Time.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    300,
		size:       want,
		chunkProps: make([]ChunkProps, want),
	}

	bar := stripansi.Strip(pb.render(p.Time.Start, &p))
	b := strings.Index(bar, "[") + 1
	e := strings.Index(bar, "]")
	got := len(bar[b:e])
	if got != want {
		t.Errorf("bar length incorrect, want %v, but got %v", want, got)
	}
}
