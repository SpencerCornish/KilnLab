package ical

import (
	"strings"
	"testing"
	"time"
)

func TestWrite(t *testing.T) {
	start := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	var b strings.Builder
	err := Write(&b, Calendar{Name: "Kiln firings", Events: []Event{{
		UID: UID("firing", 7, "kiln.example"), Summary: "Cone 6 Glaze (75% full)",
		Description: "Room left: 25%; requests close Wed, 9pm\nhttps://kiln.example/firings/7",
		URL:         "https://kiln.example/firings/7", Start: start, End: start.Add(30 * time.Hour),
		Modified: start.Add(-time.Hour), Tentative: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "UID:firing-7@kiln.example\r\n", "DTSTART:20261112T160000Z\r\n",
		"DTEND:20261113T220000Z\r\n", "SUMMARY:Cone 6 Glaze (75% full)\r\n",
		`DESCRIPTION:Room left: 25%\; requests close Wed\, 9pm\nhttps://kiln.example`, "STATUS:TENTATIVE\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("bare LF in output")
	}
}

func TestFold(t *testing.T) {
	long := "SUMMARY:" + strings.Repeat("é", 60) // 128 octets
	folded := fold(long)
	for i, l := range strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("line %d is %d octets", i, len(l))
		}
		if i > 0 && !strings.HasPrefix(l, " ") {
			t.Errorf("continuation %d lacks leading space", i)
		}
	}
	if strings.ReplaceAll(strings.TrimSuffix(folded, "\r\n"), "\r\n ", "") != long {
		t.Error("unfolding doesn't round-trip")
	}
	if fold("A:b") != "A:b\r\n" {
		t.Error("short lines unchanged")
	}
}

func TestWriteAllDay(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	var b strings.Builder
	if err := Write(&b, Calendar{Events: []Event{{
		UID: UID("firing", 8, "kiln.example"), Summary: "Glaze (60% full)",
		Start: day, End: day.AddDate(0, 0, 1), AllDay: true, Modified: day, Tentative: true,
	}}}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"DTSTART;VALUE=DATE:20261120\r\n", "DTEND;VALUE=DATE:20261121\r\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "DTSTART:") {
		t.Error("all-day event written with a time")
	}
}
