// Package ical writes a minimal RFC 5545 calendar (VEVENTs only).
package ical

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Event is one VEVENT. Times are written in UTC.
type Event struct {
	UID         string
	Summary     string
	Description string
	URL         string
	Location    string
	Start, End  time.Time
	Modified    time.Time
	Tentative   bool // STATUS:TENTATIVE instead of CONFIRMED
}

// Calendar is a feed of events.
type Calendar struct {
	Name   string
	Events []Event
}

const stamp = "20060102T150405Z"

func utc(t time.Time) string { return t.UTC().Format(stamp) }

// escape handles TEXT values: backslash, comma, semicolon and newlines.
func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(s)
}

// fold splits a content line into ≤75-octet chunks joined by CRLF + space,
// never cutting a UTF-8 sequence.
func fold(line string) string {
	const limit = 75
	if len(line) <= limit {
		return line + "\r\n"
	}
	var b strings.Builder
	width := limit
	for len(line) > width {
		cut := width
		for cut > 0 && line[cut]&0xC0 == 0x80 { // inside a multi-byte rune
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		width = limit - 1 // continuation lines start with a space
	}
	b.WriteString(line)
	b.WriteString("\r\n")
	return b.String()
}

// Write renders the calendar.
func Write(w io.Writer, c Calendar) error {
	var b strings.Builder
	line := func(name, value string) { b.WriteString(fold(name + ":" + value)) }
	line("BEGIN", "VCALENDAR")
	line("VERSION", "2.0")
	line("PRODID", "-//KilnLab//Kiln firings//EN")
	line("CALSCALE", "GREGORIAN")
	line("METHOD", "PUBLISH")
	if c.Name != "" {
		line("X-WR-CALNAME", escape(c.Name))
	}
	line("REFRESH-INTERVAL;VALUE=DURATION", "PT1H")
	line("X-PUBLISHED-TTL", "PT1H")
	for _, e := range c.Events {
		line("BEGIN", "VEVENT")
		line("UID", e.UID)
		line("DTSTAMP", utc(e.Modified))
		line("LAST-MODIFIED", utc(e.Modified))
		line("DTSTART", utc(e.Start))
		line("DTEND", utc(e.End))
		line("SUMMARY", escape(e.Summary))
		if e.Description != "" {
			line("DESCRIPTION", escape(e.Description))
		}
		if e.Location != "" {
			line("LOCATION", escape(e.Location))
		}
		if e.URL != "" {
			line("URL", e.URL)
		}
		status := "CONFIRMED"
		if e.Tentative {
			status = "TENTATIVE"
		}
		line("STATUS", status)
		line("TRANSP", "TRANSPARENT")
		line("END", "VEVENT")
	}
	line("END", "VCALENDAR")
	_, err := io.WriteString(w, b.String())
	return err
}

// UID makes a stable event id like "firing-7@kiln.example".
func UID(kind string, id int64, host string) string { return fmt.Sprintf("%s-%d@%s", kind, id, host) }
