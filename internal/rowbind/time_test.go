// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func legacyDefaultTime(text string, loc *time.Location) (value time.Time, err error) {
	for _, layout := range []string{time.RFC3339Nano, time.DateTime, time.DateOnly} {
		value, err = time.ParseInLocation(layout, text, loc)
		if err == nil {
			return
		}
	}
	return
}

func TestDefaultTimeLayoutSelection(t *testing.T) {
	for _, loc := range []*time.Location{time.UTC, time.FixedZone("offset", 8*3600)} {
		for _, text := range []string{
			"2026-09-08",
			"2026-09-08 01:02:03",
			"2026-09-08 1:02:03",
			"2026-09-08 01:02:03.123456789",
			"2026-09-08 01:02:03,123",
			"2026-09-08T01:02:03Z",
			"2026-09-08T01:02:03.123+08:00",
			"",
			"2026-02-30",
			"2026-09-08 25:02:03",
			"2026-09-08 01:02:03+08:00",
			"0000-00-00",
			"2026-09-08T01:02:03invalid",
			"1234567890 abc",
			"2026-9-8",
		} {
			want, wantErr := legacyDefaultTime(text, loc)
			got, err := (GeneralScanner{Location: loc}).scanTime(text)
			if !sameTimeParseError(err, wantErr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("%q: got %v, %v; want %v, %v", text, got, err, want, wantErr)
			}
		}
	}

	// Explicit layout order takes precedence over default format recognition.
	text := "2026-09-08"
	custom := "2006-02-01"
	want, _ := time.Parse(custom, text)
	got, err := (GeneralScanner{TimeLayouts: []string{custom, time.DateOnly}}).scanTime(text)
	if err != nil || got != want {
		t.Fatal("custom layout precedence changed", got, err)
	}
}

func FuzzDefaultTimeLayoutSelection(f *testing.F) {
	for _, s := range []string{
		"2026-09-08",
		"2026-09-08 01:02:03.123",
		"2026-09-08T01:02:03Z",
		"2026-02-30",
		"",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, text string) {
		want, wantErr := legacyDefaultTime(text, time.UTC)
		got, err := (GeneralScanner{}).scanTime(text)
		if !sameTimeParseError(err, wantErr) || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: got %v, %v; want %v, %v", text, got, err, want, wantErr)
		}
	})
}

// These are independently produced parse errors, not an error and its sentinel.
// Check the concrete error payload, including fields omitted by Error().
func sameTimeParseError(got, want error) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}

	g, ok := errors.AsType[*time.ParseError](got)
	w, wok := errors.AsType[*time.ParseError](want)
	return ok && wok && g != nil && w != nil && g.Message == w.Message &&
		g.Layout == w.Layout && g.LayoutElem == w.LayoutElem &&
		g.Value == w.Value && g.ValueElem == w.ValueElem
}
