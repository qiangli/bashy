package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"
)

const (
	probeSize   = 512 << 10
	chunkLimit  = 2 << 20
	bufferLimit = 4 << 20
	offline     = 8 * time.Second
	startup     = 10 * time.Second
)

func secret() string {
	var b [24]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

type participant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Position int    `json:"position,omitempty"`
	lease    string
	seen     time.Time
	probed   time.Time
}

type broadcast struct {
	ID, Generation uint64
	Presenter      string
	Deadline       time.Time
	Audio          bool
	Live           bool

	member *participant
	called time.Time
	mime   string
	offset uint64
	bytes  int
	chunks [][]byte
}

type session struct {
	invites         map[string]*participant
	roster          []*participant
	queue           []*participant
	assigned        *participant
	current         *broadcast
	notice          string
	started         bool
	ended           bool
	limit           int
	warning         int
	serial          uint64
	viewer          string
	viewerSeen      time.Time
	displaySettings displaySettings
}

func newSession() *session {
	return &session{invites: map[string]*participant{}, limit: 120, warning: 15}
}

func (s *session) issue() string {
	key := secret()
	s.invites[key] = &participant{ID: secret(), Status: "Joined"}
	return key
}

func (s *session) join(p *participant, name string, now time.Time) string {
	if p.lease != "" {
		s.withdraw(p, "Reconnected", now)
	}
	if p.Name == "" {
		s.roster = append(s.roster, p)
	}
	p.Name, p.lease, p.seen, p.Status = name, secret(), now, "Joined"
	return p.lease
}

func (s *session) remove(p *participant) {
	for i, queued := range s.queue {
		if queued == p {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			return
		}
	}
}

func (s *session) ready(p *participant, now time.Time) error {
	if p.probed.IsZero() || now.Sub(p.probed) > 15*time.Second {
		return errors.New("Run the connection check again.")
	}
	if p.Status != "Ready" && (s.current == nil || s.current.member != p) {
		p.Status = "Ready"
		s.queue = append(s.queue, p)
	}
	s.advance(now)
	return nil
}

func (s *session) call(p *participant, now time.Time) {
	s.remove(p)
	p.Status = "Starting"
	s.serial++
	s.current = &broadcast{ID: s.serial, Generation: 1, Presenter: p.Name, member: p, called: now}
}

func (s *session) advance(now time.Time) {
	if !s.started || s.ended || s.viewer == "" || now.Sub(s.viewerSeen) >= offline {
		return
	}
	if s.assigned != nil && s.assigned.Status == "Ready" {
		p := s.assigned
		s.assigned = nil
		if s.current != nil {
			outgoing := s.current.member
			outgoing.Status = "Ready"
			s.queue = append([]*participant{outgoing}, s.queue...)
		}
		s.call(p, now)
		return
	}
	if s.current == nil && len(s.queue) > 0 {
		s.call(s.queue[0], now)
	}
}

func (s *session) finish(status, reason string, now time.Time) {
	if s.current == nil {
		return
	}
	s.notice = s.current.Presenter + " · " + reason
	s.current.member.Status = status
	s.current.member.probed = time.Time{}
	s.current = nil
	s.advance(now)
}

func (s *session) withdraw(p *participant, status string, now time.Time) {
	s.remove(p)
	p.probed = time.Time{}
	if s.current != nil && s.current.member == p {
		s.finish(status, status, now)
		return
	}
	p.Status = status
}

func (s *session) tick(now time.Time) {
	for _, p := range s.roster {
		if now.Sub(p.seen) >= offline && p.Status != "Offline" {
			s.remove(p)
			p.Status, p.probed = "Offline", time.Time{}
			if s.assigned == p {
				s.assigned = nil
			}
		}
	}
	if s.current != nil {
		t := s.current
		switch {
		case t.member.Status == "Offline":
			s.finish("Offline", "connection lost", now)
		case !t.Deadline.IsZero() && !now.Before(t.Deadline):
			s.finish("Finished", "time is up", now)
		case t.Deadline.IsZero() && now.Sub(t.called) >= startup:
			s.finish("Missed", "no projected video", now)
		}
	}
}

func (t *broadcast) resetMedia() {
	t.Generation++
	t.mime, t.Audio, t.Live = "", false, false
	t.offset, t.bytes, t.chunks = 0, 0, nil
}

func (t *broadcast) put(seq uint64, mime string, chunk []byte) error {
	if seq != t.offset+uint64(len(t.chunks)) || len(chunk)+t.bytes > bufferLimit || len(t.chunks) >= 32 {
		return errors.New("Playback fell behind. Prepare again to retry.")
	}
	if t.mime != "" && mime != t.mime {
		return errors.New("Media format changed.")
	}
	t.mime = mime
	t.Audio = mime == "video/webm;codecs=vp8,opus"
	t.bytes += len(chunk)
	t.chunks = append(t.chunks, chunk)
	return nil
}

func (t *broadcast) ack(seq uint64) {
	for t.offset < seq && len(t.chunks) > 0 {
		t.bytes -= len(t.chunks[0])
		t.chunks[0] = nil
		t.chunks = t.chunks[1:]
		t.offset++
	}
}
