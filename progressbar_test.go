package p0d

import (
	"testing"
	"time"
)

func TestProgressBarMarkRamp(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	p := P0d{}
	p.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")
	p.Stop, _ = time.Parse(layout, "2022-01-01T00:00:30.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    30,
		size:       20,
		chunkProps: make([]ChunkProps, 20),
	}

	pn := p.Start.Add(3 * time.Second)
	pb.markRamp(pn, &p)

	if pb.chunkProps[pb.chunkPropIndexFor(pn, &p)].isRamp == false {
		t.Error("ramp not set")
	}
}

func TestProgressBarMarkError(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	p := P0d{}
	p.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")
	p.Stop, _ = time.Parse(layout, "2022-01-01T00:00:30.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    30,
		size:       20,
		chunkProps: make([]ChunkProps, 20),
	}

	pn := p.Start.Add(3 * time.Second)
	pb.markError(pn, &p)

	if pb.chunkProps[pb.chunkPropIndexFor(pn, &p)].hasErrors == false {
		t.Error("error not set")
	}
}

func TestProgressBarChunkIndex(t *testing.T) {
	layout := "2006-01-02T15:04:05.000Z"
	p := P0d{}
	p.Start, _ = time.Parse(layout, "2022-01-01T00:00:00.000Z")
	p.Stop, _ = time.Parse(layout, "2022-01-01T00:00:30.000Z")

	pb := ProgressBar{
		curSecs:    0,
		maxSecs:    30,
		size:       20,
		chunkProps: make([]ChunkProps, 20),
	}

	type chunkTest struct {
		deltaSecond float64
		wantIndex   int
	}

	tests := []chunkTest{
		{deltaSecond: 0.0, wantIndex: 0},
		{deltaSecond: 1.0, wantIndex: 0},
		{deltaSecond: 2.0, wantIndex: 1},
		{deltaSecond: 3.0, wantIndex: 2},
		{deltaSecond: 4.0, wantIndex: 2},
		{deltaSecond: 4.4, wantIndex: 2},
		{deltaSecond: 4.5, wantIndex: 3},
		{deltaSecond: 5.0, wantIndex: 3},
		{deltaSecond: 6.0, wantIndex: 4},
		{deltaSecond: 7.0, wantIndex: 4},
		{deltaSecond: 7.5, wantIndex: 5},
		{deltaSecond: 7.6, wantIndex: 5},
		{deltaSecond: 8.0, wantIndex: 5},
		{deltaSecond: 8.9, wantIndex: 5},
		{deltaSecond: 9.01, wantIndex: 6},
		{deltaSecond: 27.0, wantIndex: 18},
		{deltaSecond: 28.5, wantIndex: 19},
		{deltaSecond: 29.9, wantIndex: 19},
		{deltaSecond: 30.0, wantIndex: 20},
		{deltaSecond: 30.5, wantIndex: 20},
	}

	for _, tc := range tests {
		pns := p.Start.Add(time.Duration(tc.deltaSecond * float64(time.Second)))
		got := pb.chunkPropIndexFor(pns, &p)
		if got != tc.wantIndex {
			t.Errorf("bad chunk index for deltaSeconds %v, want %v got %v", tc.deltaSecond, tc.wantIndex, got)
		}
	}
}
