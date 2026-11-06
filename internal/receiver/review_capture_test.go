// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ColonelBlimp/go-rtty/internal/reference"
	"github.com/ColonelBlimp/go-rtty/rtty"
)

type reviewedClip struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Bytes       int    `json:"bytes"`
	SampleRate  int    `json:"sample_rate"`
	SampleCount int    `json:"sample_count"`
}

type reviewedPlan struct {
	Schema int    `json:"schema_version"`
	Kind   string `json:"kind"`
	Config struct {
		MarkHz          float64 `json:"mark_hz"`
		SpaceHz         float64 `json:"space_hz"`
		BaudNumerator   uint32  `json:"baud_numerator"`
		BaudDenominator uint32  `json:"baud_denominator"`
		StopHalfBits    uint8   `json:"stop_half_bits"`
		Alphabet        string  `json:"alphabet"`
	} `json:"configuration"`
	ConfigHash   string         `json:"configuration_sha256"`
	SampleRate   int            `json:"sample_rate"`
	InitialState string         `json:"initial_state"`
	USOS         []string       `json:"rx_usos"`
	Clips        []reviewedClip `json:"clips"`
}

type reviewedResult struct {
	Decoder      string        `json:"decoder"`
	Window       string        `json:"window"`
	USOS         string        `json:"rx_usos"`
	WAVHash      string        `json:"wav_sha256"`
	Text         string        `json:"text"`
	Codes        []int         `json:"codes"` // []uint8 would encode as base64.
	Observations []Observation `json:"observations"`
}

func reviewRead(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("review input exceeds %d bytes", limit)
	}
	return data, err
}

// reviewPath resolves existing ancestors too, so new output paths cannot escape
// the external cache through a symlink. The cache itself must already exist.
func reviewPath(cache, name string) (string, error) {
	root, err := filepath.EvalSymlinks(cache)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	ancestor := path
	var missing []string
	for {
		_, err = os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) || ancestor == filepath.Dir(ancestor) {
			return "", fmt.Errorf("resolve review path: %w", err)
		}
		missing = append(missing, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
	ancestor, err = filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		ancestor = filepath.Join(ancestor, missing[i])
	}
	rel, err := filepath.Rel(root, ancestor)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("review path escapes external cache")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		return "", err
	}
	repo, err = filepath.EvalSymlinks(repo)
	if err != nil {
		return "", err
	}
	rel, err = filepath.Rel(repo, root)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return "", fmt.Errorf("review cache must be outside repository")
	}
	rel, err = filepath.Rel(repo, ancestor)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return "", fmt.Errorf("review artifact must be outside repository")
	}
	return ancestor, nil
}

func reviewedConfig(plan reviewedPlan) rtty.Config {
	cfg := rtty.DefaultConfig()
	cfg.SampleRate, cfg.CenterHz, cfg.ShiftHz = plan.SampleRate, (plan.Config.MarkHz+plan.Config.SpaceHz)/2, math.Abs(plan.Config.MarkHz-plan.Config.SpaceHz)
	cfg.Polarity = rtty.MarkHigh
	if plan.Config.MarkHz < plan.Config.SpaceHz {
		cfg.Polarity = rtty.MarkLow
	}
	cfg.StopBits = rtty.StopBits(plan.Config.StopHalfBits)
	cfg.Baud = rtty.BaudRate{Numerator: plan.Config.BaudNumerator, Denominator: plan.Config.BaudDenominator}
	return cfg
}

func reviewDecode(t testing.TB, cfg rtty.Config, p noiseParameters, guarded, usos bool, pcm []int16, chunk int) []Observation {
	t.Helper()
	var decoder interface {
		Process([]int16, []Observation) []Observation
	}
	if guarded {
		r, err := newNoiseTrial(cfg, usos, "guarded-offset", p)
		if err != nil {
			t.Fatal(err)
		}
		decoder = &shortNoiseTrial{r}
	} else {
		decoder = newReceiver(t, cfg, usos)
	}
	var obs []Observation
	for pos := 0; pos < len(pcm); pos += chunk {
		obs = decoder.Process(pcm[pos:min(pos+chunk, len(pcm))], obs)
	}
	return obs
}

// TestReviewedCorpusCapture is an explicit external-data driver, not a normal
// corpus test. scripts/transcripts.py preflights the admission and plan. This
// decoder sees configuration and audio only; it does not read any labels.
func TestReviewedCorpusCapture(t *testing.T) {
	planName, outputName := os.Getenv("RTTY_REVIEW_PLAN"), os.Getenv("RTTY_REVIEW_OUTPUT")
	if planName == "" && outputName == "" {
		t.Skip("external reviewed-corpus capture not requested")
	}
	cache := os.Getenv("RTTY_CORPUS")
	if cache == "" || planName == "" || outputName == "" {
		t.Fatal("RTTY_CORPUS, RTTY_REVIEW_PLAN and RTTY_REVIEW_OUTPUT are all required")
	}
	planPath, err := reviewPath(cache, planName)
	if err != nil {
		t.Fatal(err)
	}
	output, err := reviewPath(cache, outputName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("capture output already exists or cannot be inspected")
	}
	data, err := reviewRead(planPath, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	var plan reviewedPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Schema != 1 || plan.Kind != "reviewed-capture-plan" || plan.InitialState != "fresh-letters" ||
		!reflect.DeepEqual(plan.USOS, []string{"off", "on"}) || len(plan.Clips) == 0 || len(plan.Clips) > 128 || plan.Config.Alphabet != "US" {
		t.Fatal("unsupported capture plan")
	}
	p, params := loadNoise(t)
	sources := map[string]string{"testdata/manifests/receiver-noise-aware-v1.json": fmt.Sprintf("%x", sha256.Sum256(params))}
	for _, name := range []string{"review_capture_test.go", "receiver.go", "framer.go", "text.go", "receiver_test.go", "noise_study_test.go", "noise_trial_test.go", "noise_short_test.go", "threshold_test.go", "../../rtty/config.go", "../../rtty/alphabet.go", "../reference/wav.go"} {
		b, err := reviewRead(name, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		sources[filepath.ToSlash(filepath.Clean("internal/receiver/"+name))] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	var results []reviewedResult
	seen := make(map[string]bool)
	for _, clip := range plan.Clips {
		if seen[clip.ID] || clip.ID == "" || clip.SampleRate != plan.SampleRate ||
			(clip.SampleRate != 12000 && clip.SampleRate != 48000) || clip.SampleCount <= 0 ||
			clip.SampleCount > 120*clip.SampleRate || clip.Bytes != 44+2*clip.SampleCount {
			t.Fatal("invalid clip metadata")
		}
		seen[clip.ID] = true
		path, err := reviewPath(cache, clip.Path)
		if err != nil {
			t.Fatal(err)
		}
		wav, err := reviewRead(path, int64(clip.Bytes))
		if err != nil || len(wav) != clip.Bytes || fmt.Sprintf("%x", sha256.Sum256(wav)) != clip.SHA256 {
			t.Fatalf("clip checksum/size mismatch: %s: %v", clip.ID, err)
		}
		rate, pcm, err := reference.DecodeWAV(wav)
		if err != nil || rate != clip.SampleRate || len(pcm) != clip.SampleCount {
			t.Fatalf("invalid clip WAV: %v", err)
		}
		cfg := reviewedConfig(plan)
		for _, candidate := range []string{"go-ratio", "go-guarded-short"} {
			for _, mode := range plan.USOS {
				obs := reviewDecode(t, cfg, p, candidate == "go-guarded-short", mode == "on", pcm, 257)
				whole := reviewDecode(t, cfg, p, candidate == "go-guarded-short", mode == "on", pcm, len(pcm))
				if !reflect.DeepEqual(obs, whole) {
					t.Fatal("capture differs with PCM partitioning")
				}
				codes, text := codesAndText(obs)
				integers := make([]int, len(codes))
				for i, c := range codes {
					integers[i] = int(c)
				}
				results = append(results, reviewedResult{candidate, clip.ID, mode, clip.SHA256, text, integers, obs})
			}
		}
	}
	report := struct {
		Schema     int               `json:"schema_version"`
		Kind       string            `json:"kind"`
		Status     string            `json:"status"`
		PlanHash   string            `json:"plan_sha256"`
		ConfigHash string            `json:"configuration_sha256"`
		GoVersion  string            `json:"go_version"`
		Sources    map[string]string `json:"sources_sha256"`
		Results    []reviewedResult  `json:"results"`
	}{1, "reviewed-capture", "captured-unscored", fmt.Sprintf("%x", sha256.Sum256(data)), plan.ConfigHash, runtime.Version(), sources, results}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		t.Fatal(err)
	}
	temp, err := os.CreateTemp(filepath.Dir(output), ".review-capture-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(encoded, '\n')); err != nil {
		temp.Close()
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(temp.Name(), output); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedCaptureKnownTransmitters(t *testing.T) {
	p, _ := loadNoise(t)
	for _, f := range fixtures(t) {
		var plan reviewedPlan
		plan.SampleRate = f.SampleRate
		plan.Config.MarkHz, plan.Config.SpaceHz = f.MarkHz, f.SpaceHz
		plan.Config.BaudNumerator, plan.Config.BaudDenominator = f.BaudNumerator, f.BaudDenominator
		plan.Config.StopHalfBits, plan.Config.Alphabet = uint8(f.StopHalfBits), "US"
		cfg := reviewedConfig(plan)
		if cfg != f.Config {
			t.Fatalf("capture config differs from independently specified fixture %s", f.ID)
		}
		for _, guarded := range []bool{false, true} {
			for _, usos := range []bool{false, true} {
				obs := reviewDecode(t, cfg, p, guarded, usos, f.PCM, 257)
				codes, text := codesAndText(obs)
				want := f.ExpectedOff
				if usos {
					want = f.ExpectedOn
				}
				if !reflect.DeepEqual(codes, f.ExpectedCodes) || text != want {
					t.Fatalf("%s guarded=%v usos=%v: codes %v text %q", f.ID, guarded, usos, codes, text)
				}
				whole := reviewDecode(t, cfg, p, guarded, usos, f.PCM, len(f.PCM))
				if !reflect.DeepEqual(obs, whole) {
					t.Fatalf("partitioned capture differs: %s", f.ID)
				}
			}
		}
	}
}

func TestReviewedCapturePathsAndLimits(t *testing.T) {
	cache := t.TempDir()
	if _, err := reviewPath(cache, "new/subdir/capture.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewPath(cache, "../escape.json"); err == nil {
		t.Fatal("accepted path outside cache")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cache, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewPath(cache, "escape/capture.json"); err == nil {
		t.Fatal("accepted symlink outside cache")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewPath(repo, "README.md"); err == nil {
		t.Fatal("accepted repository cache")
	}
	if _, err := reviewPath(string(os.PathSeparator), filepath.Join(repo, "README.md")); err == nil {
		t.Fatal("accepted repository artifact under an ancestor cache")
	}
	file := filepath.Join(cache, "bounded.json")
	if err := os.WriteFile(file, []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewRead(file, 4); err == nil {
		t.Fatal("accepted input exceeding limit")
	}
}
