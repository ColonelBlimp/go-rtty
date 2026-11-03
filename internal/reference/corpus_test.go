// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package reference

import (
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"testing"
)

type corpusAudio struct {
	Path        string `json:"path"`
	URL         string `json:"url,omitempty"`
	SHA256      string `json:"sha256"`
	Bytes       int    `json:"bytes"`
	SampleRate  int    `json:"sample_rate"`
	SampleCount int    `json:"sample_count"`
}

// Only the manifest is in the repository. This test neither fetches nor requires
// external audio, a resampler, or a GUI; corpus:verify checks cached payloads.
func TestExternalCorpusManifest(t *testing.T) {
	var m struct {
		SchemaVersion        int    `json:"schema_version"`
		Description          string `json:"description"`
		SourcePage           string `json:"source_page"`
		RetrievedUTCDate     string `json:"retrieved_utc_date"`
		RedistributionStatus string `json:"redistribution_status"`
		PartitionPolicy      string `json:"partition_policy"`
		SettingsBasis        string `json:"settings_basis"`
		Conversion           struct {
			Tool                string         `json:"tool"`
			LibrarySHA256       string         `json:"library_sha256"`
			Settings            map[string]any `json:"settings"`
			ConfigurationSHA256 string         `json:"configuration_sha256"`
			BoundaryMapping     string         `json:"boundary_mapping"`
			DelayPolicy         string         `json:"delay_policy"`
			ClippingPolicy      string         `json:"clipping_policy"`
		} `json:"conversion"`
		ReferenceProfile struct {
			BaudNumerator   int     `json:"baud_numerator"`
			BaudDenominator int     `json:"baud_denominator"`
			ShiftHz         int     `json:"shift_hz"`
			DataBits        int     `json:"data_bits"`
			StartBits       int     `json:"start_bits"`
			Alphabet        string  `json:"alphabet"`
			AFC             bool    `json:"afc"`
			Squelch         bool    `json:"squelch"`
			RXUSOS          []bool  `json:"rx_usos"`
			TXUSOS          bool    `json:"tx_usos"`
			BandwidthHz     int     `json:"bandwidth_hz"`
			FilterFactor    float64 `json:"filter_factor"`
		} `json:"reference_profile"`
		Fixtures []struct {
			ID                string            `json:"id"`
			Target            string            `json:"target"`
			Condition         string            `json:"condition"`
			RecordingGroup    string            `json:"recording_group"`
			Partition         string            `json:"partition"`
			LabelStatus       string            `json:"label_status"`
			ScoredRanges      []json.RawMessage `json:"scored_ranges"`
			Original          corpusAudio       `json:"original"`
			Derived           []corpusAudio     `json:"derived"`
			ReferenceSettings struct {
				CarrierHz    int `json:"carrier_hz"`
				MarkHz       int `json:"mark_hz"`
				SpaceHz      int `json:"space_hz"`
				StopHalfBits int `json:"stop_half_bits"`
			} `json:"reference_settings"`
		} `json:"fixtures"`
	}
	load(t, "manifests/gritty.json", &m)
	if m.SchemaVersion != 1 || len(m.Fixtures) != 5 || m.SourcePage != "https://dxatlas.com/Gritty/Demo/" ||
		m.RetrievedUTCDate == "" || m.RedistributionStatus == "" || m.PartitionPolicy == "" || m.SettingsBasis == "" {
		t.Fatal("incomplete corpus provenance")
	}
	var tools toolsManifest
	load(t, "manifests/tools.json", &tools)
	matched := false
	for _, tool := range tools.Tools {
		if tool.Name == m.Conversion.Tool && tool.ExecutableSHA256 == m.Conversion.LibrarySHA256 {
			matched = true
		}
	}
	configuration, err := json.Marshal(m.Conversion.Settings)
	if !matched || err != nil || hash(configuration) != m.Conversion.ConfigurationSHA256 ||
		m.Conversion.BoundaryMapping == "" || m.Conversion.DelayPolicy == "" || m.Conversion.ClippingPolicy == "" {
		t.Fatal("unpinned converter or missing timing/clipping policy")
	}
	p := m.ReferenceProfile
	if p.BaudNumerator != 909 || p.BaudDenominator != 20 || p.ShiftHz != 170 || p.DataBits != 5 || p.StartBits != 1 ||
		p.Alphabet != "US" || p.AFC || p.Squelch || p.TXUSOS || p.BandwidthHz != 68 || p.FilterFactor != 1.25 ||
		len(p.RXUSOS) != 2 || p.RXUSOS[0] || !p.RXUSOS[1] {
		t.Fatal("reference profile differs from the qualified replay adapter")
	}
	validHash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	validID := regexp.MustCompile(`^[a-z0-9_-]+$`)
	seen := map[string]bool{}
	groups := map[string]string{}
	paths := map[string]bool{}
	for _, f := range m.Fixtures {
		if !validID.MatchString(f.ID) || seen[f.ID] || f.Target == "" || f.Condition == "" || f.RecordingGroup == "" {
			t.Fatalf("invalid or duplicate identity: %s", f.ID)
		}
		seen[f.ID] = true
		if f.LabelStatus != "unlabelled" || len(f.ScoredRanges) != 0 || f.Partition != "development" {
			t.Fatalf("%s: inspected unlabelled audio must not become scored/held-out evidence", f.ID)
		}
		if partition, ok := groups[f.RecordingGroup]; ok && partition != f.Partition {
			t.Fatalf("recording group split across partitions: %s", f.RecordingGroup)
		}
		groups[f.RecordingGroup] = f.Partition
		u, err := url.Parse(f.Original.URL)
		if err != nil || u.Scheme != "https" || u.Host != "dxatlas.com" || path.Ext(u.Path) != ".wav" ||
			f.Original.SampleRate != 6000 || f.Original.SampleCount <= 0 || len(f.Derived) != 2 {
			t.Fatalf("invalid source format: %s", f.ID)
		}
		settings := f.ReferenceSettings
		if settings.SpaceHz-settings.MarkHz != p.ShiftHz || settings.CarrierHz*2 != settings.MarkHz+settings.SpaceHz ||
			settings.MarkHz <= 0 || settings.SpaceHz >= 3000 || settings.StopHalfBits != 3 {
			t.Fatalf("invalid reference tuning/framing: %s", f.ID)
		}
		for _, a := range append([]corpusAudio{f.Original}, f.Derived...) {
			if paths[a.Path] || path.IsAbs(a.Path) || path.Clean(a.Path) != a.Path ||
				!validHash.MatchString(a.SHA256) || a.Bytes < a.SampleCount*2+44 {
				t.Fatalf("invalid artifact: %s", a.Path)
			}
			paths[a.Path] = true
		}
		if f.Original.Path != "original/"+f.ID+".wav" {
			t.Fatalf("unexpected original cache path: %s", f.ID)
		}
		for i, rate := range []int{12000, 48000} {
			d := f.Derived[i]
			if path.Dir(d.Path) != "prepared" || d.URL != "" || d.SampleRate != rate ||
				d.SampleCount != f.Original.SampleCount*(rate/6000) || d.Bytes != 44+d.SampleCount*2 {
				t.Fatalf("derived audio loses timing/format provenance: %s", d.Path)
			}
		}
	}
}
