// RFC 5545 VCALENDAR / VEVENT subset reader and writer (stdlib only; no RRULE).
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// FormatICS serializes events as a single RFC 5545 VCALENDAR stream.
func FormatICS(events []*event) []byte {
	var b bytes.Buffer
	writeICSLine(&b, "BEGIN:VCALENDAR")
	writeICSLine(&b, "VERSION:2.0")
	writeICSLine(&b, "PRODID:-//bashy//desk-calendar//EN")
	writeICSLine(&b, "CALSCALE:GREGORIAN")

	for _, ev := range events {
		writeICSLine(&b, "BEGIN:VEVENT")
		uid := ev.Source.UID
		if uid == "" {
			uid = ev.ID + "@bashy.calendar"
		}
		writeICSLine(&b, "UID:"+escapeICSText(uid))

		stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if t, err := time.Parse(time.RFC3339, ev.Updated); err == nil {
			stamp = t.UTC()
		} else if t, err := time.Parse(time.RFC3339, ev.Created); err == nil {
			stamp = t.UTC()
		}
		writeICSLine(&b, "DTSTAMP:"+stamp.Format("20060102T150405Z"))

		startT, _ := time.Parse(time.RFC3339, ev.Start)
		endT, _ := time.Parse(time.RFC3339, ev.End)
		loc, locErr := validateTZ(ev.TZ)
		if locErr != nil || ev.TZ == "UTC" {
			writeICSLine(&b, "DTSTART:"+startT.UTC().Format("20060102T150405Z"))
			writeICSLine(&b, "DTEND:"+endT.UTC().Format("20060102T150405Z"))
		} else {
			writeICSLine(&b, "DTSTART;TZID="+ev.TZ+":"+startT.In(loc).Format("20060102T150405"))
			writeICSLine(&b, "DTEND;TZID="+ev.TZ+":"+endT.In(loc).Format("20060102T150405"))
		}

		writeICSLine(&b, "SUMMARY:"+escapeICSText(ev.Title))
		if ev.Body != "" {
			writeICSLine(&b, "DESCRIPTION:"+escapeICSText(ev.Body))
		}
		if ev.Location != "" {
			writeICSLine(&b, "LOCATION:"+escapeICSText(ev.Location))
		}
		switch ev.Status {
		case StatusCancelled:
			writeICSLine(&b, "STATUS:CANCELLED")
		default:
			writeICSLine(&b, "STATUS:CONFIRMED")
		}
		writeICSLine(&b, "X-BASHY-ID:"+escapeICSText(ev.ID))
		writeICSLine(&b, "X-BASHY-STATUS:"+escapeICSText(ev.Status))
		if len(ev.Audience) > 0 {
			writeICSLine(&b, "X-BASHY-AUDIENCE:"+escapeICSText(strings.Join(ev.Audience, ",")))
		}
		for _, ref := range ev.Refs {
			if strings.TrimSpace(ref) != "" {
				writeICSLine(&b, "URL:"+strings.TrimSpace(ref))
			}
		}
		writeICSLine(&b, "END:VEVENT")
	}

	writeICSLine(&b, "END:VCALENDAR")
	return b.Bytes()
}

// writeICSLine folds a property line at <= 75 UTF-8 octets per RFC 5545 §3.1.
func writeICSLine(b *bytes.Buffer, line string) {
	if len(line) <= 75 {
		b.WriteString(line)
		b.WriteString("\r\n")
		return
	}
	first := true
	rem := line
	for len(rem) > 0 {
		limit := 75
		if !first {
			limit = 74 // leading space takes 1 octet
		}
		if len(rem) <= limit {
			if !first {
				b.WriteByte(' ')
			}
			b.WriteString(rem)
			b.WriteString("\r\n")
			break
		}
		cut := limit
		for cut > 0 && !utf8.RuneStart(rem[cut]) {
			cut--
		}
		if cut == 0 {
			cut = limit
		}
		if !first {
			b.WriteByte(' ')
		}
		b.WriteString(rem[:cut])
		b.WriteString("\r\n")
		rem = rem[cut:]
		first = false
	}
}

func escapeICSText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case ';':
			b.WriteString(`\;`)
		case ',':
			b.WriteString(`\,`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unescapeICSText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			case '\\':
				b.WriteByte('\\')
			case ';':
				b.WriteByte(';')
			case ',':
				b.WriteByte(',')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ParseICS parses an RFC 5545 VCALENDAR stream into upsertable event specs.
func ParseICS(raw []byte) ([]upsertSpec, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	rawLines := strings.Split(text, "\n")

	// Unfold continuation lines starting with SPACE or HTAB.
	var lines []string
	for _, ln := range rawLines {
		if len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t') {
			if len(lines) == 0 {
				return nil, errors.New("invalid ICS: folded line before first property")
			}
			lines[len(lines)-1] += ln[1:]
			continue
		}
		if strings.TrimSpace(ln) == "" {
			continue
		}
		lines = append(lines, ln)
	}

	inCal := false
	seenCal := false
	inEvent := false
	inNested := 0 // ignore VALARM / VTIMEZONE subcomponents inside VCALENDAR/VEVENT

	type rawEvent struct {
		uid         string
		bashyID     string
		summary     string
		description string
		location    string
		status      string
		bashyStatus string
		audience    []string
		refs        []string
		dtStartVal  string
		dtStartP    map[string]string
		dtEndVal    string
		dtEndP      map[string]string
	}
	var cur rawEvent
	var out []upsertSpec

	for idx, ln := range lines {
		name, params, val, err := splitICSProp(ln)
		if err != nil {
			return nil, fmt.Errorf("ICS line %d: %w", idx+1, err)
		}
		if name == "BEGIN" {
			comp := strings.ToUpper(strings.TrimSpace(val))
			switch {
			case comp == "VCALENDAR" && !inCal:
				inCal = true
				seenCal = true
			case comp == "VEVENT" && inCal && !inEvent && inNested == 0:
				inEvent = true
				cur = rawEvent{refs: []string{}, audience: []string{}}
			case inCal:
				inNested++
			default:
				return nil, fmt.Errorf("ICS line %d: unexpected BEGIN:%s outside VCALENDAR", idx+1, comp)
			}
			continue
		}
		if name == "END" {
			comp := strings.ToUpper(strings.TrimSpace(val))
			switch {
			case inNested > 0:
				inNested--
			case comp == "VEVENT" && inEvent:
				spec, err := buildSpecFromICS(cur)
				if err != nil {
					return nil, err
				}
				out = append(out, spec)
				inEvent = false
			case comp == "VCALENDAR" && inCal && !inEvent:
				inCal = false
			default:
				return nil, fmt.Errorf("ICS line %d: unmatched END:%s", idx+1, comp)
			}
			continue
		}
		if !inEvent || inNested > 0 {
			continue
		}
		switch name {
		case "RRULE", "RDATE", "EXDATE":
			return nil, fmt.Errorf("recurrence (%s) is not supported in calendar MVP", name)
		case "UID":
			cur.uid = strings.TrimSpace(unescapeICSText(val))
		case "X-BASHY-ID":
			cur.bashyID = strings.TrimSpace(unescapeICSText(val))
		case "X-BASHY-STATUS":
			cur.bashyStatus = strings.ToLower(strings.TrimSpace(unescapeICSText(val)))
		case "X-BASHY-AUDIENCE":
			for _, p := range strings.Split(unescapeICSText(val), ",") {
				if p = strings.TrimSpace(p); p != "" {
					cur.audience = appendUnique(cur.audience, p)
				}
			}
		case "SUMMARY":
			cur.summary = unescapeICSText(val)
		case "DESCRIPTION":
			cur.description = unescapeICSText(val)
		case "LOCATION":
			cur.location = unescapeICSText(val)
		case "STATUS":
			cur.status = strings.ToUpper(strings.TrimSpace(val))
		case "URL":
			u := strings.TrimSpace(val)
			if u != "" {
				cur.refs = appendUnique(cur.refs, u)
			}
		case "DTSTART":
			cur.dtStartVal = strings.TrimSpace(val)
			cur.dtStartP = params
		case "DTEND":
			cur.dtEndVal = strings.TrimSpace(val)
			cur.dtEndP = params
		}
	}

	if !seenCal {
		return nil, errors.New("invalid ICS: missing VCALENDAR")
	}
	if inEvent || inCal {
		return nil, errors.New("invalid ICS: unclosed VCALENDAR or VEVENT")
	}
	return out, nil
}

func splitICSProp(line string) (string, map[string]string, string, error) {
	// Find the first unquoted colon separating property+params from value.
	inQuote := false
	colon := -1
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			inQuote = !inQuote
		} else if line[i] == ':' && !inQuote {
			colon = i
			break
		}
	}
	if colon <= 0 {
		return "", nil, "", errors.New("malformed property (missing ':')")
	}
	head := line[:colon]
	val := line[colon+1:]

	parts := splitOutsideQuotes(head, ';')
	name := strings.ToUpper(strings.TrimSpace(parts[0]))
	params := map[string]string{}
	for _, p := range parts[1:] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.Trim(strings.TrimSpace(v), `"`)
		params[k] = v
	}
	return name, params, val, nil
}

func splitOutsideQuotes(s string, sep byte) []string {
	var out []string
	inQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
		} else if s[i] == sep && !inQuote {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func buildSpecFromICS(raw struct {
	uid         string
	bashyID     string
	summary     string
	description string
	location    string
	status      string
	bashyStatus string
	audience    []string
	refs        []string
	dtStartVal  string
	dtStartP    map[string]string
	dtEndVal    string
	dtEndP      map[string]string
}) (upsertSpec, error) {
	if raw.dtStartVal == "" {
		return upsertSpec{}, errors.New("VEVENT missing required DTSTART")
	}
	startT, tz, isDate, err := parseICSDateTime(raw.dtStartVal, raw.dtStartP, "")
	if err != nil {
		return upsertSpec{}, fmt.Errorf("VEVENT DTSTART: %w", err)
	}
	var endT time.Time
	if raw.dtEndVal == "" {
		if isDate {
			endT = startT.AddDate(0, 0, 1)
		} else {
			endT = startT.Add(time.Hour)
		}
	} else {
		var endErr error
		endT, _, _, endErr = parseICSDateTime(raw.dtEndVal, raw.dtEndP, tz)
		if endErr != nil {
			return upsertSpec{}, fmt.Errorf("VEVENT DTEND: %w", endErr)
		}
	}
	if endT.Before(startT) {
		return upsertSpec{}, errors.New("VEVENT DTEND is before DTSTART")
	}
	loc, err := validateTZ(tz)
	if err != nil {
		return upsertSpec{}, err
	}

	uid := raw.uid
	if uid == "" {
		sum := sha256.Sum256([]byte(raw.summary + "|" + raw.dtStartVal))
		uid = "ics-" + hex.EncodeToString(sum[:8])
	}
	title := strings.TrimSpace(raw.summary)
	if title == "" {
		title = "Untitled event"
	}

	status := "pending"
	if raw.status == "CANCELLED" {
		status = StatusCancelled
	} else if validEventStatus(raw.bashyStatus) {
		status = raw.bashyStatus
	}

	return upsertSpec{
		ID:       raw.bashyID,
		Title:    title,
		Start:    startT.In(loc).Format(time.RFC3339),
		End:      endT.In(loc).Format(time.RFC3339),
		TZ:       tz,
		Location: raw.location,
		Body:     raw.description,
		Refs:     raw.refs,
		Audience: raw.audience,
		Status:   status,
		Source:   eventSource{Provider: "ics", UID: uid},
	}, nil
}

func parseICSDateTime(val string, params map[string]string, fallbackTZ string) (time.Time, string, bool, error) {
	tzid := params["TZID"]
	valType := strings.ToUpper(params["VALUE"])

	// All-day DATE value: YYYYMMDD
	if valType == "DATE" || (len(val) == 8 && !strings.Contains(val, "T")) {
		tz := tzid
		if tz == "" {
			if fallbackTZ != "" {
				tz = fallbackTZ
			} else {
				tz = "UTC"
			}
		}
		loc, err := validateTZ(tz)
		if err != nil {
			return time.Time{}, "", false, err
		}
		t, err := time.ParseInLocation("20060102", val, loc)
		if err != nil {
			return time.Time{}, "", false, fmt.Errorf("invalid DATE %q", val)
		}
		return t, tz, true, nil
	}

	// UTC DATE-TIME ending in Z
	if strings.HasSuffix(val, "Z") {
		if tzid != "" {
			if _, err := validateTZ(tzid); err != nil {
				return time.Time{}, "", false, err
			}
		}
		t, err := time.Parse("20060102T150405Z", val)
		if err != nil {
			return time.Time{}, "", false, fmt.Errorf("invalid UTC DATE-TIME %q", val)
		}
		tz := "UTC"
		if tzid != "" {
			tz = tzid
		} else if fallbackTZ != "" {
			tz = fallbackTZ
		}
		loc, err := validateTZ(tz)
		if err != nil {
			return time.Time{}, "", false, err
		}
		return t.In(loc), tz, false, nil
	}

	// Local or TZID-qualified DATE-TIME: YYYYMMDDTHHMMSS
	tz := tzid
	if tz == "" {
		if fallbackTZ != "" {
			tz = fallbackTZ
		} else {
			tz = "UTC"
		}
	}
	loc, err := validateTZ(tz)
	if err != nil {
		return time.Time{}, "", false, err
	}
	t, err := time.ParseInLocation("20060102T150405", val, loc)
	if err != nil {
		// Also accept RFC3339 if an extended client wrote it.
		if t2, err2 := time.Parse(time.RFC3339, val); err2 == nil {
			return t2.In(loc), tz, false, nil
		}
		return time.Time{}, "", false, fmt.Errorf("invalid DATE-TIME %q", val)
	}
	return t, tz, false, nil
}
