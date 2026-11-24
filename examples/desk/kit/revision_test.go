package kit

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testClock(start time.Time) func() time.Time {
	n := 0
	return func() time.Time {
		n++
		return start.Add(time.Duration(n) * time.Second)
	}
}

func TestRevisionSaveAndItemFileIsolation(t *testing.T) {
	dir := t.TempDir()
	itemFile := filepath.Join(dir, "items", "agenda.json")
	if err := AtomicWrite(itemFile, []byte(`{"id":"agenda","votes":{"a":1}}`)); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	s := RevisionStore{Dir: dir, now: testClock(t0)}

	first, err := s.Save("agenda", "", "", "ana@example.test (member)", "genie", []byte("# Agenda v1"))
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if first.Revision != "r1" || first.Active != "r1" || first.Previous != "" {
		t.Fatalf("first state = %+v, want revision/active r1 and empty previous", first)
	}
	if first.Author != "ana@example.test (member)" || first.Assistant != "genie" || !first.Created.Equal(t0.Add(time.Second)) {
		t.Fatalf("first metadata = %+v", first)
	}

	second, err := s.SaveRevision(SaveInput{
		Item:    "agenda",
		BaseRev: "r1",
		Author:  "Guest Ana",
		Kind:    "anonymous",
		Data:    []byte("# Agenda v2"),
	})
	if err != nil {
		t.Fatalf("second save against r1: %v", err)
	}
	if second.Revision != "r2" || second.Active != "r2" || second.Previous != "r1" || second.Kind != "anonymous" {
		t.Fatalf("second state = %+v, want active r2 previous r1 kind anonymous", second)
	}

	got1, rev1, err := s.Read("agenda", "", "r1")
	if err != nil || string(got1) != "# Agenda v1" || rev1.Author != "ana@example.test (member)" || rev1.Assistant != "genie" || rev1.Kind != "" {
		t.Fatalf("read r1 = (%q, %+v, %v)", got1, rev1, err)
	}
	got2, rev2, err := s.Read("agenda", "", "r2")
	if err != nil || string(got2) != "# Agenda v2" || rev2.Author != "Guest Ana" || rev2.Kind != "anonymous" || rev2.Assistant != "" {
		t.Fatalf("read r2 = (%q, %+v, %v)", got2, rev2, err)
	}

	// The store never owns or modifies the app's item file.
	rawItem, err := os.ReadFile(itemFile)
	if err != nil || string(rawItem) != `{"id":"agenda","votes":{"a":1}}` {
		t.Fatalf("item file modified: %q (%v)", rawItem, err)
	}
}

func TestRevisionStaleBaseReturnsBothRevisions(t *testing.T) {
	dir := t.TempDir()
	s := RevisionStore{Dir: dir}

	// Stale base on an item with no active revision yet returns both ("r1" and "").
	_, err := s.Save("note1", "", "r1", "ana", "", []byte("draft"))
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("save with base on empty store: err = %v, want *RevisionConflictError", err)
	}
	if conflict.BaseRev != "r1" || conflict.Active != "" {
		t.Fatalf("empty-store conflict = %+v, want base_rev=r1 active=\"\"", conflict)
	}

	if _, err := s.Save("note1", "", "", "ana", "", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("note1", "", "r1", "bob", "", []byte("v2")); err != nil {
		t.Fatal(err)
	}

	// Saving against r1 while r2 is active refuses and returns both r1 and r2.
	cur, err := s.Save("note1", "", "r1", "ana", "", []byte("stale edit"))
	if !errors.As(err, &conflict) {
		t.Fatalf("stale save: err = %v, want *RevisionConflictError", err)
	}
	if conflict.BaseRev != "r1" || conflict.Active != "r2" {
		t.Fatalf("conflict = %+v, want base_rev=r1 active=r2", conflict)
	}
	if !strings.Contains(err.Error(), "r1") || !strings.Contains(err.Error(), "r2") {
		t.Fatalf("conflict error %q must name both r1 and r2", err.Error())
	}
	if cur.Active != "r2" || cur.Previous != "r1" {
		t.Fatalf("returned state on stale save = %+v, want active=r2 previous=r1", cur)
	}

	// Verify HTTP 409 conflict helper under ContractHandler carries both revisions.
	rec := httptest.NewRecorder()
	ContractHandler("page", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !WriteRevisionConflict(w, err) {
			t.Fatal("WriteRevisionConflict did not match")
		}
	})).ServeHTTP(rec, httptest.NewRequest("POST", "/api/action", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("HTTP status = %d, want 409", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 409 JSON: %v", err)
	}
	if body["base_rev"] != "r1" || body["active"] != "r2" {
		t.Fatalf("409 JSON must include both base_rev and active: %v", body)
	}
}

func TestRevisionSaveWithoutBaseOverwritesAndRecords(t *testing.T) {
	dir := t.TempDir()
	s := RevisionStore{Dir: dir}

	if _, err := s.Save("doc", "", "", "ana", "", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("doc", "", "r1", "bob", "", []byte("second")); err != nil {
		t.Fatal(err)
	}
	// No base_rev is a recorded overwrite even when r2 is active.
	third, err := s.Save("doc", "", "", "carol", "helper", []byte("third"))
	if err != nil {
		t.Fatalf("save without base_rev: %v", err)
	}
	if third.Revision != "r3" || third.Active != "r3" || third.Previous != "r2" {
		t.Fatalf("third = %+v, want r3 active and r2 previous", third)
	}
	// Prior snapshots remain intact and readable.
	if b, _, err := s.Read("doc", "", "r2"); err != nil || string(b) != "second" {
		t.Fatalf("r2 snapshot = %q (%v), want \"second\"", b, err)
	}
}

func TestRevisionUndoSwapsActiveAndPrevious(t *testing.T) {
	dir := t.TempDir()
	s := RevisionStore{Dir: dir}

	if _, _, err := s.Undo("doc", ""); !errors.Is(err, ErrNoPrevious) {
		t.Fatalf("undo empty store = %v, want ErrNoPrevious", err)
	}
	if _, err := s.Save("doc", "", "", "ana", "", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Undo("doc", ""); !errors.Is(err, ErrNoPrevious) {
		t.Fatalf("undo single revision = %v, want ErrNoPrevious", err)
	}
	if _, err := s.Save("doc", "", "r1", "bob", "", []byte("two")); err != nil {
		t.Fatal(err)
	}

	u1, data1, err := s.Undo("doc", "")
	if err != nil {
		t.Fatalf("first undo: %v", err)
	}
	if u1.Active != "r1" || u1.Previous != "r2" || u1.Revision != "r1" || u1.Author != "ana" || string(data1) != "one" {
		t.Fatalf("after first undo: state=%+v data=%q", u1, data1)
	}

	// Second undo swaps active and previous back again.
	u2, data2, err := s.Undo("doc", "")
	if err != nil {
		t.Fatalf("second undo: %v", err)
	}
	if u2.Active != "r2" || u2.Previous != "r1" || u2.Revision != "r2" || u2.Author != "bob" || string(data2) != "two" {
		t.Fatalf("after second undo: state=%+v data=%q", u2, data2)
	}
}

func TestRevisionRestoreAnyAndSaveAfterRestore(t *testing.T) {
	dir := t.TempDir()
	s := RevisionStore{Dir: dir}

	for _, step := range []struct {
		base, author, text string
	}{
		{"", "ana", "body-1"},
		{"r1", "bob", "body-2"},
		{"r2", "carol", "body-3"},
	} {
		if _, err := s.Save("page1", "content", step.base, step.author, "", []byte(step.text)); err != nil {
			t.Fatal(err)
		}
	}

	if _, _, err := s.Restore("page1", "content", "r99"); !errors.Is(err, ErrUnknownRevision) {
		t.Fatalf("restore unknown = %v, want ErrUnknownRevision", err)
	}

	// Restore r1 (not merely previous r2).
	res, data, err := s.Restore("page1", "content", "r1")
	if err != nil {
		t.Fatalf("restore r1: %v", err)
	}
	if res.Active != "r1" || res.Previous != "r3" || res.Revision != "r1" || res.Author != "ana" || string(data) != "body-1" {
		t.Fatalf("after restore r1: state=%+v data=%q", res, data)
	}

	// Restoring the already-active revision is a no-op that preserves Previous.
	noop, dataNoop, err := s.Restore("page1", "content", "r1")
	if err != nil || noop.Active != "r1" || noop.Previous != "r3" || string(dataNoop) != "body-1" {
		t.Fatalf("restore active noop: state=%+v data=%q err=%v", noop, dataNoop, err)
	}

	// Save against pre-restore tip r3 is stale now that r1 is active.
	var conflict *RevisionConflictError
	if _, err := s.Save("page1", "content", "r3", "dave", "", []byte("stale")); !errors.As(err, &conflict) || conflict.BaseRev != "r3" || conflict.Active != "r1" {
		t.Fatalf("save against r3 after restore r1: err=%v conflict=%+v", err, conflict)
	}

	// Save against restored active r1 mints r4 while keeping r2 and r3 intact.
	fourth, err := s.Save("page1", "content", "r1", "dave", "agent-x", []byte("body-4"))
	if err != nil {
		t.Fatalf("save after restore: %v", err)
	}
	if fourth.Revision != "r4" || fourth.Active != "r4" || fourth.Previous != "r1" {
		t.Fatalf("fourth = %+v, want revision/active r4 and previous r1", fourth)
	}
	for rev, want := range map[string]string{"r1": "body-1", "r2": "body-2", "r3": "body-3", "r4": "body-4"} {
		got, _, err := s.Read("page1", "content", rev)
		if err != nil || string(got) != want {
			t.Fatalf("retained %s = %q (%v), want %q", rev, got, err, want)
		}
	}
}

func TestRevisionHistoryOrderAndReloadFromDisk(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	writer := RevisionStore{Dir: dir, now: testClock(t0)}

	// Empty history before any save is clean, never an error.
	empty, err := writer.History("memo1", "")
	if err != nil || empty.Active != "" || empty.Previous != "" || len(empty.Versions) != 0 {
		t.Fatalf("empty history = %+v (%v)", empty, err)
	}

	if _, err := writer.SaveRevision(SaveInput{Item: "memo1", Author: "ana@example.test", Kind: "member", Data: []byte("v1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.SaveRevision(SaveInput{Item: "memo1", BaseRev: "r1", Author: "Bob", Kind: "anonymous", Assistant: "claude", Data: []byte("v2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.SaveRevision(SaveInput{Item: "memo1", BaseRev: "r2", Author: "guest-1234", Kind: "account", Data: []byte("v3")}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writer.Restore("memo1", "", "r2"); err != nil {
		t.Fatal(err)
	}

	// Reopen a fresh RevisionStore against the same directory (reload from disk).
	reloaded := RevisionStore{Dir: dir, now: testClock(t0.Add(time.Minute))}
	h, err := reloaded.History("memo1", "")
	if err != nil {
		t.Fatalf("reloaded history: %v", err)
	}
	if h.Active != "r2" || h.Previous != "r3" {
		t.Fatalf("reloaded pointers = active %q previous %q, want r2 / r3", h.Active, h.Previous)
	}
	if len(h.Versions) != 3 {
		t.Fatalf("versions count = %d, want 3: %+v", len(h.Versions), h.Versions)
	}
	// History lists newest-first: r3, r2, r1.
	wantOrder := []string{"r3", "r2", "r1"}
	for i, want := range wantOrder {
		if h.Versions[i].Revision != want {
			t.Fatalf("history[%d] = %+v, want revision %s", i, h.Versions[i], want)
		}
	}
	if h.Versions[0].Kind != "account" || h.Versions[1].Author != "Bob" || h.Versions[1].Kind != "anonymous" || h.Versions[1].Assistant != "claude" || h.Versions[2].Kind != "member" {
		t.Fatalf("reloaded kind/author metadata = %+v", h.Versions)
	}
	if h.Versions[0].Assistant != "" || h.Versions[2].Created.IsZero() {
		t.Fatalf("unexpected metadata in %+v", h.Versions)
	}

	// Continuing to save on the reloaded store enforces active r2 and mints r4.
	if _, err := reloaded.Save("memo1", "", "r3", "ana@example.test", "", []byte("stale")); err == nil {
		t.Fatal("reloaded store accepted stale base r3 while r2 is active")
	}
	fourth, err := reloaded.Save("memo1", "", "r2", "ana@example.test", "", []byte("v4"))
	if err != nil || fourth.Revision != "r4" || fourth.Active != "r4" || fourth.Previous != "r2" {
		t.Fatalf("save after reload = %+v (%v)", fourth, err)
	}
}

func TestRevisionInterruptedSaveRecoversCleanly(t *testing.T) {
	dir := t.TempDir()
	s := RevisionStore{Dir: dir}

	if _, err := s.Save("doc", "", "", "ana", "", []byte("v1")); err != nil {
		t.Fatal(err)
	}

	// Manually leave unpublished r2 orphan files on disk without advancing state.json.
	partRoot := filepath.Join(dir, "revisions", "doc", "_")
	if err := AtomicWrite(filepath.Join(partRoot, "snapshots", "r2"), []byte("manual-orphan-v2")); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(filepath.Join(partRoot, "versions", "r2.json"), storedVersion{
		Seq:      2,
		Revision: "r2",
		Digest:   sha256Hex([]byte("manual-orphan-v2")),
		Author:   "ghost",
		Created:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	// Unpublished r2 orphan must not appear in History, Read or Restore.
	h, err := s.History("doc", "")
	if err != nil || h.Active != "r1" || h.Previous != "" || len(h.Versions) != 1 {
		t.Fatalf("history with orphan r2 = %+v (%v), want only r1", h, err)
	}
	if _, _, err := s.Read("doc", "", "r2"); !errors.Is(err, ErrUnknownRevision) {
		t.Fatalf("read orphan r2 = %v, want ErrUnknownRevision", err)
	}
	if _, _, err := s.Restore("doc", "", "r2"); !errors.Is(err, ErrUnknownRevision) {
		t.Fatalf("restore orphan r2 = %v, want ErrUnknownRevision", err)
	}

	// Next Save with different bytes atomically replaces the unpublished r2 orphan.
	second, err := s.Save("doc", "", "r1", "carol", "", []byte("committed-v2"))
	if err != nil {
		t.Fatalf("save replacing orphan r2 failed: %v", err)
	}
	if second.Revision != "r2" || second.Active != "r2" || second.Previous != "r1" {
		t.Fatalf("second = %+v, want r2 active and r1 previous", second)
	}
	if got, rev, err := s.Read("doc", "", "r2"); err != nil || string(got) != "committed-v2" || rev.Author != "carol" {
		t.Fatalf("read r2 = (%q, %+v, %v)", got, rev, err)
	}

	// Also verify an in-flight interruption before state.json leaves r3 recoverable.
	boom := errors.New("interrupted before state.json")
	interrupted := RevisionStore{Dir: dir, beforePublish: func() error { return boom }}
	if _, err := interrupted.Save("doc", "", "r2", "bob", "", []byte("aborted-v3")); !errors.Is(err, boom) {
		t.Fatalf("interrupted save err = %v, want %v", err, boom)
	}
	third, err := s.Save("doc", "", "r2", "bob", "", []byte("committed-v3"))
	if err != nil || third.Revision != "r3" {
		t.Fatalf("third save after interruption = %+v (%v)", third, err)
	}

	// Listed versions stay immutable: r1 and r2 still hold their committed bytes.
	for rev, want := range map[string]string{"r1": "v1", "r2": "committed-v2", "r3": "committed-v3"} {
		if got, _, err := s.Read("doc", "", rev); err != nil || string(got) != want {
			t.Fatalf("listed %s = %q (%v), want %q", rev, got, err, want)
		}
	}
}

func TestRevisionSharedActionHandlerAndTamperGuard(t *testing.T) {
	dir := t.TempDir()
	s := RevisionStore{Dir: dir}
	var applied string
	var failApply bool

	handler := ContractHandler("notepad", s.ActionHandler(func(st RevisionState, data []byte) error {
		if failApply {
			return errors.New("disk full")
		}
		applied = string(data)
		return nil
	}))

	post := func(payload string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/action", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
		return rec.Code, out
	}

	if _, err := s.SaveRevision(SaveInput{Item: "n1", Author: "ana@example.test", Kind: "member", Assistant: "genie", Data: []byte("alpha")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveRevision(SaveInput{Item: "n1", BaseRev: "r1", Author: "Bob", Kind: "anonymous", Data: []byte("beta")}); err != nil {
		t.Fatal(err)
	}

	code, hist := post(`{"action":"history","id":"n1"}`)
	if code != http.StatusOK || hist["schema_version"] != "bashy-notepad-v1" || hist["active"] != "r2" || hist["previous"] != "r1" {
		t.Fatalf("history = %d %v", code, hist)
	}
	versions, _ := hist["versions"].([]any)
	if len(versions) != 2 {
		t.Fatalf("history versions = %v", hist["versions"])
	}

	code, undone := post(`{"action":"undo","id":"n1"}`)
	if code != http.StatusOK || undone["active"] != "r1" || undone["previous"] != "r2" || undone["revision"] != "r1" || undone["kind"] != "member" || applied != "alpha" {
		t.Fatalf("undo = %d %v applied=%q", code, undone, applied)
	}

	// A failing apply callback rolls the pointer back so item and store match.
	failApply = true
	code, _ = post(`{"action":"restore","id":"n1","revision":"r2"}`)
	failApply = false
	if code != http.StatusInternalServerError {
		t.Fatalf("failed apply status = %d, want 500", code)
	}
	if h, err := s.History("n1", ""); err != nil || h.Active != "r1" || h.Previous != "r2" {
		t.Fatalf("pointer moved despite failed apply: %+v (%v)", h, err)
	}

	code, restored := post(`{"action":"restore","id":"n1","revision":"r2"}`)
	if code != http.StatusOK || restored["active"] != "r2" || restored["previous"] != "r1" || restored["kind"] != "anonymous" || applied != "beta" {
		t.Fatalf("restore = %d %v applied=%q", code, restored, applied)
	}

	if code, _ := post(`{"action":"restore","id":"n1","revision":"r99"}`); code != http.StatusNotFound {
		t.Fatalf("restore unknown status = %d, want 404", code)
	}
	if code, _ := post(`{"action":"restore","id":"n1"}`); code != http.StatusBadRequest {
		t.Fatalf("restore missing revision status = %d, want 400", code)
	}

	// Confine item, part and revision names; tampered snapshot bytes fail closed.
	for _, bad := range []string{"", "..", "../escape", "/abs", "a\\b"} {
		if _, err := s.Save(bad, "", "", "ana", "", []byte("x")); err == nil {
			t.Errorf("invalid item %q accepted", bad)
		}
		if bad != "" {
			if _, err := s.Save("n1", bad, "", "ana", "", []byte("x")); err == nil {
				t.Errorf("invalid part %q accepted", bad)
			}
		}
	}
	snap := filepath.Join(dir, "revisions", "n1", "_", "snapshots", "r1")
	if err := os.WriteFile(snap, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Read("n1", "", "r1"); err == nil {
		t.Fatal("tampered snapshot accepted by Read")
	}
	if _, _, err := s.Undo("n1", ""); err == nil {
		t.Fatal("tampered snapshot accepted by Undo")
	}
}
