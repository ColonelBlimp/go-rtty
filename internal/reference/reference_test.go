// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package reference

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

const dataRoot = "../../testdata/"

func read(t *testing.T, name string) []byte {
	t.Helper()
	r, err := os.OpenRoot(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	f, err := r.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16<<20+1))
	if err != nil || len(b) > 16<<20 {
		t.Fatalf("read %s: too large or unreadable: %v", name, err)
	}
	return b
}

func load(t *testing.T, name string, v any) {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(read(t, name)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		t.Fatalf("%s: trailing content", name)
	}
}

func hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func checkHash(t *testing.T, name, want string) []byte {
	t.Helper()
	b := read(t, name)
	if got := hash(b); got != want {
		t.Fatalf("%s: SHA-256 = %s, want %s", name, got, want)
	}
	return b
}

type artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type tool struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	License          string   `json:"license"`
	Platform         string   `json:"platform"`
	Package          artifact `json:"package"`
	Source           artifact `json:"source"`
	Executable       string   `json:"executable"`
	ExecutableSHA256 string   `json:"executable_sha256"`
}

type toolsManifest struct {
	SchemaVersion   int        `json:"schema_version"`
	Tools           []tool     `json:"tools"`
	RuntimePackages []artifact `json:"runtime_packages"`
}

func TestToolPins(t *testing.T) {
	var m toolsManifest
	load(t, "manifests/tools.json", &m)
	if m.SchemaVersion != 1 || len(m.Tools) != 3 {
		t.Fatal("unexpected reference-tool manifest")
	}
	validHash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	seen := map[string]bool{}
	for _, tool := range m.Tools {
		if seen[tool.Name] || tool.Name == "" || tool.Version == "" || tool.License == "" ||
			tool.Platform != "linux-amd64" || !validHash.MatchString(tool.ExecutableSHA256) {
			t.Fatalf("invalid tool pin: %s", tool.Name)
		}
		seen[tool.Name] = true
		for _, a := range []artifact{tool.Package, tool.Source} {
			if !strings.HasPrefix(a.URL, "https://") || !validHash.MatchString(a.SHA256) {
				t.Fatalf("invalid artifact pin: %+v", a)
			}
		}
	}
	if !seen["minimodem"] || !seen["fldigi"] || !seen["samplerate"] {
		t.Fatal("missing required reference tool")
	}
	for _, a := range m.RuntimePackages {
		if !strings.HasPrefix(a.URL, "https://") || !validHash.MatchString(a.SHA256) {
			t.Fatalf("invalid runtime artifact pin: %+v", a)
		}
	}
}

type fixture struct {
	ID                  string   `json:"id"`
	SampleRate          int      `json:"sample_rate"`
	MarkHz              int      `json:"mark_hz"`
	SpaceHz             int      `json:"space_hz"`
	StopHalfBits        int      `json:"stop_half_bits"`
	InputPath           string   `json:"input_path"`
	ExpectedCodes       []int    `json:"expected_codes"`
	ExpectedTextUSOSOn  string   `json:"expected_text_usos_on"`
	ExpectedTextUSOSOff string   `json:"expected_text_usos_off"`
	Path                string   `json:"path"`
	BaudNumerator       int      `json:"baud_numerator"`
	BaudDenominator     int      `json:"baud_denominator"`
	StartBits           int      `json:"start_bits"`
	DataBits            int      `json:"data_bits"`
	Alphabet            string   `json:"alphabet"`
	TXUSOS              bool     `json:"tx_usos"`
	RXUSOS              bool     `json:"rx_usos"`
	Amplitude           float64  `json:"amplitude"`
	SineTableLength     int      `json:"sine_table_length"`
	UncertainSpans      []string `json:"uncertain_spans"`
	TXArgs              []string `json:"tx_args"`
	RXArgs              []string `json:"rx_args"`
	SHA256              string   `json:"sha256"`
	SampleCount         int      `json:"sample_count"`
	Timing              struct {
		Model            string `json:"model"`
		BitSamples       int    `json:"bit_samples"`
		StopSamples      int    `json:"stop_samples"`
		LeadSamples      int    `json:"lead_samples"`
		TailSamples      int    `json:"tail_samples"`
		TrimStartSamples int    `json:"trim_start_samples"`
		TrimEndSamples   int    `json:"trim_end_samples"`
	} `json:"timing"`
	CapturePath         string `json:"capture_path"`
	CaptureSHA256       string `json:"capture_sha256"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
}

func TestIndependentFixtures(t *testing.T) {
	var m struct {
		SchemaVersion   int       `json:"schema_version"`
		Description     string    `json:"description"`
		Generator       string    `json:"generator"`
		GeneratorSHA256 string    `json:"generator_sha256"`
		License         string    `json:"license"`
		Fixtures        []fixture `json:"fixtures"`
	}
	load(t, "manifests/independent.json", &m)
	var pins toolsManifest
	load(t, "manifests/tools.json", &pins)
	if m.SchemaVersion != 1 || len(m.Fixtures) != 9 || m.License != "GPL-3.0-only" ||
		m.Generator != "minimodem" || m.GeneratorSHA256 != pins.Tools[0].ExecutableSHA256 {
		t.Fatal("unexpected independent fixture manifest")
	}
	seen := map[string]bool{}
	for _, f := range m.Fixtures {
		t.Run(f.ID, func(t *testing.T) {
			if seen[f.ID] || f.ID == "" {
				t.Fatal("empty or duplicate fixture ID")
			}
			seen[f.ID] = true
			b := checkHash(t, f.Path, f.SHA256)
			rate, samples, err := inspectWAV(b)
			if err != nil || rate != f.SampleRate || samples != f.SampleCount {
				t.Fatalf("WAV metadata mismatch: %d Hz, %d samples, %v", rate, samples, err)
			}
			if f.BaudNumerator != 909 || f.BaudDenominator != 20 || f.StartBits != 1 || f.DataBits != 5 ||
				f.Alphabet != "US" || !f.TXUSOS || !f.RXUSOS || f.Amplitude != .5 || f.SineTableLength != 4096 ||
				(f.StopHalfBits != 3 && f.StopHalfBits != 4) ||
				!((f.MarkHz == 1585 && f.SpaceHz == 1415) || (f.MarkHz == 1415 && f.SpaceHz == 1585)) ||
				len(f.UncertainSpans) != 0 {
				t.Fatal("unexpected reference profile")
			}
			clock := f.Timing
			if clock.BitSamples != rate*22/1000 || clock.StopSamples != clock.BitSamples*f.StopHalfBits/2 ||
				clock.LeadSamples != 2*clock.BitSamples || clock.TailSamples != 2*clock.BitSamples ||
				clock.TrimStartSamples != 0 || clock.TrimEndSamples != 0 ||
				samples != clock.LeadSamples+len(f.ExpectedCodes)*(6*clock.BitSamples+clock.StopSamples)+clock.TailSamples {
				t.Fatal("unexpected minimodem sample schedule or scoring trim")
			}
			if string(read(t, f.InputPath)) != f.ExpectedTextUSOSOn {
				t.Fatal("input differs from exact USOS-on text")
			}
			settings, err := json.Marshal([][]string{f.TXArgs, f.RXArgs})
			if err != nil || hash(settings) != f.ConfigurationSHA256 {
				t.Fatal("configuration checksum mismatch")
			}
			common := []string{"-5", "--startbits", "1", "--stopbits", fmt.Sprintf("%.1f", float64(f.StopHalfBits)/2),
				"-R", strconv.Itoa(rate), "-M", strconv.Itoa(f.MarkHz), "-S", strconv.Itoa(f.SpaceHz)}
			wantTX := append(append([]string{"--tx"}, common...), "--volume", "0.5", "--lut", "4096", "-f", "{wav}", "45.45")
			wantRX := append(append([]string{"--rx"}, common...), "--bandwidth", "10", "--confidence", "1.5", "--limit", "2.3", "-f", "{wav}", "45.45")
			if !slices.Equal(f.TXArgs, wantTX) || !slices.Equal(f.RXArgs, wantRX) {
				t.Fatal("recorded commands disagree with fixture settings")
			}
			checkHash(t, f.CapturePath, f.CaptureSHA256)
			var capture struct {
				Text           string   `json:"text"`
				DataLSBFirst   []string `json:"data_lsb_first"`
				Diagnostics    string   `json:"diagnostics"`
				RawDiagnostics string   `json:"raw_diagnostics"`
			}
			load(t, f.CapturePath, &capture)
			if capture.Text != f.ExpectedTextUSOSOn || len(capture.DataLSBFirst) != len(f.ExpectedCodes) {
				t.Fatal("captured text or raw-code count mismatch")
			}
			for i, bits := range capture.DataLSBFirst {
				if f.ExpectedCodes[i] > 31 || f.ExpectedCodes[i] < 0 || len(bits) != 5 {
					t.Fatal("invalid raw code")
				}
				for bit := range 5 {
					if bits[bit] != byte('0'+(f.ExpectedCodes[i]>>bit)&1) {
						t.Fatalf("raw code %d does not match captured wire bits", i)
					}
				}
			}
		})
	}
}

func TestProtocolFixtures(t *testing.T) {
	rows, err := csv.NewReader(bytes.NewReader(read(t, "protocol/alphabet-us.csv"))).ReadAll()
	if err != nil || len(rows) != 33 {
		t.Fatalf("alphabet: expected header and all 32 codes: %v", err)
	}
	for code, row := range rows[1:] {
		if len(row) != 4 || row[0] != strconv.Itoa(code) || row[1] == "" || row[2] == "" || len(row[3]) != 5 {
			t.Fatalf("invalid alphabet row %d", code)
		}
		for bit := range 5 {
			if row[3][bit] != byte('0'+(code>>bit)&1) {
				t.Fatalf("code %d wire bits are not LSB-first", code)
			}
		}
	}
	var frames struct {
		SchemaVersion int    `json:"schema_version"`
		Description   string `json:"description"`
		Cases         []struct {
			Code             int    `json:"code"`
			StopHalfBits     int    `json:"stop_half_bits"`
			Levels           string `json:"levels"`
			BoundaryHalfBits []int  `json:"boundary_half_bits"`
		} `json:"cases"`
	}
	load(t, "protocol/framing.json", &frames)
	if frames.SchemaVersion != 1 || len(frames.Cases) != 8 {
		t.Fatal("unexpected framing vectors")
	}
	for _, f := range frames.Cases {
		if f.Code < 0 || f.Code > 31 || (f.StopHalfBits != 3 && f.StopHalfBits != 4) {
			t.Fatal("invalid framing profile")
		}
		want := "00"
		for _, bit := range rows[f.Code+1][3] {
			want += strings.Repeat(string(bit), 2)
		}
		want += strings.Repeat("1", f.StopHalfBits)
		if f.Levels != want || !slices.Equal(f.BoundaryHalfBits, []int{0, 2, 4, 6, 8, 10, 12, 12 + f.StopHalfBits}) {
			t.Fatalf("inconsistent frame for code %d", f.Code)
		}
	}
}

func TestTextVectorFormat(t *testing.T) {
	var m struct {
		SchemaVersion int    `json:"schema_version"`
		Description   string `json:"description"`
		Cases         []struct {
			Name       string `json:"name"`
			Input      string `json:"input"`
			Normalized string `json:"normalized"`
			Codes      []int  `json:"codes"`
		} `json:"cases"`
		Invalid []struct {
			Input      string `json:"input"`
			ByteOffset int    `json:"byte_offset"`
			Rune       rune   `json:"rune"`
		} `json:"invalid"`
	}
	load(t, "protocol/text.json", &m)
	if m.SchemaVersion != 1 || len(m.Cases) != 8 || len(m.Invalid) != 4 {
		t.Fatal("unexpected text vectors")
	}
	seen := map[string]bool{}
	for _, c := range m.Cases {
		if c.Name == "" || seen[c.Name] || c.Normalized != strings.ToUpper(c.Input) {
			t.Fatalf("invalid text vector %q", c.Name)
		}
		seen[c.Name] = true
		if (c.Input == "" && len(c.Codes) != 0) || (c.Input != "" && (len(c.Codes) == 0 || c.Codes[0] != 31)) {
			t.Fatalf("invalid initial shift in %s", c.Name)
		}
		for _, code := range c.Codes {
			if code < 0 || code > 31 {
				t.Fatalf("invalid code in %s", c.Name)
			}
		}
	}
	for _, c := range m.Invalid {
		if c.ByteOffset < 0 || c.ByteOffset >= len(c.Input) {
			t.Fatal("invalid error byte offset")
		}
		if r, _ := utf8.DecodeRuneInString(c.Input[c.ByteOffset:]); r != c.Rune {
			t.Fatal("error rune disagrees with byte offset")
		}
	}
}

func TestArtifactChecksums(t *testing.T) {
	var m struct {
		SchemaVersion int               `json:"schema_version"`
		Files         map[string]string `json:"files"`
	}
	load(t, "manifests/artifacts.json", &m)
	if m.SchemaVersion != 1 || len(m.Files) < 5 {
		t.Fatal("missing artifact inventory")
	}
	for name, sum := range m.Files {
		checkHash(t, name, sum)
	}
}

// This accepts SemVer, including build metadata, but rejects leading zeroes in
// numeric prerelease identifiers. It does not create tags or imply a release.
func validVersion(s string) bool {
	pattern := `^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if m == nil {
		return false
	}
	for _, id := range strings.Split(m[5], ".") {
		if len(id) > 1 && id[0] == '0' && strings.Trim(id, "0123456789") == "" {
			return false
		}
	}
	return true
}

func TestVersion(t *testing.T) {
	b, err := os.ReadFile("../../version.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimSuffix(string(b), "\n")
	if !validVersion(s) {
		t.Fatalf("invalid version.txt: %q", s)
	}
	for _, s := range []string{"0.1.0-dev.0", "1.2.3", "1.0.0-alpha.1+build.01"} {
		if !validVersion(s) {
			t.Errorf("rejected valid SemVer %q", s)
		}
	}
	for _, s := range []string{"v0.1.0", "01.2.3", "1.2.3-01", "1.2", "1.2.3\n", "1.2.3-", "1.2.3-alpha..1"} {
		if validVersion(s) {
			t.Errorf("accepted invalid SemVer %q", s)
		}
	}
}
