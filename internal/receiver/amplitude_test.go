// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"math"
	"reflect"
	"slices"
	"testing"
)

func TestAmplitudeStepPositions(t *testing.T) {
	m, _ := loadChannelManifest(t)
	improvements, oldErrors := 0, 0
	for _, f := range fixtures(t) {
		for _, start := range []int{31, 35, 39} {
			for _, end := range []int{61, 65, 69} {
				pcm := make([]int16, len(f.PCM))
				for i, s := range f.PCM {
					gain := m.SignalGain
					if i >= len(pcm)*start/100 && i < len(pcm)*end/100 {
						gain *= .1
					}
					pcm[i] = int16(math.RoundToEven(float64(s) * gain))
				}
				old, err := newWithTiming(f.Config, false, false)
				if err != nil {
					t.Fatal(err)
				}
				before, _ := codesAndText(old.Process(pcm, nil))
				a := edits(f.ExpectedCodes, before).total()
				oldErrors += a
				for _, usos := range []bool{false, true} {
					current := newReceiver(t, f.Config, usos)
					codes, text := codesAndText(current.Process(pcm, nil))
					want := f.ExpectedOff
					if usos {
						want = f.ExpectedOn
					}
					if !slices.Equal(codes, f.ExpectedCodes) || text != want {
						t.Fatalf("%s step %d/%d USOS%v: %v %q", f.ID, start, end, usos, codes, text)
					}
				}
				if a > 0 {
					improvements++
				}
			}
		}
	}
	t.Logf("81 step-position cases: legacy code errors %d -> 0; improved cases %d", oldErrors, improvements)
}

func TestResetDuringAmplitudeStep(t *testing.T) {
	f := fixtures(t)[8] // The independently generated USOS probe.
	m, _ := loadChannelManifest(t)
	pcm, _ := channelPCM(f, channelCase{FadeDB: 20}, m, 0)
	r := newReceiver(t, f.Config, false)
	r.Process(pcm[:len(pcm)*35/100+f.Timing.Bit], nil)
	if r.edgeHold == 0 {
		t.Fatal("test did not reach adaptive timing state")
	}
	r.Reset()
	fresh := newReceiver(t, f.Config, false)
	if !reflect.DeepEqual(r, fresh) {
		t.Fatal("reset retained adaptive state")
	}
	got := r.Process(pcm, nil)
	want := fresh.Process(pcm, nil)
	if !slices.Equal(got, want) || !reflect.DeepEqual(r, fresh) {
		t.Fatal("reset changed subsequent output/state")
	}
}

func TestAdaptiveQuietPath(t *testing.T) {
	for _, f := range fixtures(t) {
		current := newReceiver(t, f.Config, false)
		legacy, err := newWithTiming(f.Config, false, false)
		if err != nil {
			t.Fatal(err)
		}
		got := current.Process(f.PCM, nil)
		want := legacy.Process(f.PCM, nil)
		if !slices.Equal(got, want) || current.Diagnostics != legacy.Diagnostics {
			t.Fatalf("%s: steady-gain observations or counters changed", f.ID)
		}
	}
}
