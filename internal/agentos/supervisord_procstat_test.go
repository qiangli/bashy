// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"
)

// fakeProc serves a /proc tree from files (a path ending in / is an empty
// directory), failing any path listed in errs.
func fakeProc(files map[string]string, errs map[string]error) procFS {
	m := fstest.MapFS{}
	for path, data := range files {
		name := strings.Trim(path, "/")
		if strings.HasSuffix(path, "/") {
			m[name] = &fstest.MapFile{Mode: fs.ModeDir | 0o555}
			continue
		}
		m[name] = &fstest.MapFile{Data: []byte(data)}
	}
	return procFS{
		readFile: func(path string) ([]byte, error) {
			if err, ok := errs[path]; ok {
				return nil, err
			}
			return fs.ReadFile(m, strings.TrimPrefix(path, "/"))
		},
		readDir: func(path string) ([]fs.DirEntry, error) {
			if err, ok := errs[path]; ok {
				return nil, err
			}
			return fs.ReadDir(m, strings.TrimPrefix(path, "/"))
		},
	}
}

// statLine is a /proc stat line: state (3), pgrp (5), num_threads (20) and
// starttime (22).
func statLine(pid int, state string, pgrp int, threads string, start int) string {
	return fmt.Sprintf("%d (a) b) %s 1 %d %d 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 %s 0 %d 0 0", pid, state, pgrp, pgrp, threads, start)
}

const testBoot = "/proc/sys/kernel/random/boot_id"

// Liveness has three answers. Only a vanished entry or a counted-out thread
// group proves exit; EACCES, EIO and malformed data stay unknown, and the
// task directory — unavailable after the main thread exits — can keep a
// group live but never prove it gone.
func TestProcStartTimeProvesExitOnlyFromVanishingOrThreadCount(t *testing.T) {
	const live, gone, unknown = "live", "gone", "unknown"
	leader := "/proc/7/stat"
	task := "/proc/7/task/"
	cases := []struct {
		name  string
		files map[string]string
		errs  map[string]error
		want  string
	}{
		{"running leader", map[string]string{leader: statLine(7, "S", 7, "4", 900)}, nil, live},
		{"vanished stat", nil, map[string]error{leader: fs.ErrNotExist}, gone},
		{"reaped mid-read", nil, map[string]error{leader: syscall.ESRCH}, gone},
		{"stat EACCES", nil, map[string]error{leader: syscall.EACCES}, unknown},
		{"stat EIO", nil, map[string]error{leader: syscall.EIO}, unknown},
		{"stat without comm", map[string]string{leader: "7 a S 1 7"}, nil, unknown},
		{"stat truncated", map[string]string{leader: "7 (a) S 1 7 7 0"}, nil, unknown},
		{"boot id EACCES", map[string]string{leader: statLine(7, "S", 7, "1", 900)}, map[string]error{testBoot: syscall.EACCES}, unknown},
		// Main thread exited, siblings still counted: live even though the
		// task directory is unavailable.
		{"zombie leader with threads, no task dir", map[string]string{leader: statLine(7, "Z", 7, "3", 900)},
			map[string]error{"/proc/7/task": fs.ErrNotExist}, live},
		{"zombie leader, num_threads malformed", map[string]string{leader: statLine(7, "Z", 7, "x", 900)}, nil, unknown},
		{"zombie leader, num_threads negative", map[string]string{leader: statLine(7, "Z", 7, "-1", 900)}, nil, unknown},
		{"zombie alone, task dir missing", map[string]string{leader: statLine(7, "Z", 7, "1", 900)},
			map[string]error{"/proc/7/task": fs.ErrNotExist}, gone},
		{"zombie alone, task dir empty", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task: ""}, nil, gone},
		{"zombie being released", map[string]string{leader: statLine(7, "X", 7, "0", 900)},
			map[string]error{"/proc/7/task": fs.ErrNotExist}, gone},
		{"zombie alone, task dir EACCES", map[string]string{leader: statLine(7, "Z", 7, "1", 900)},
			map[string]error{"/proc/7/task": syscall.EACCES}, unknown},
		{"zombie alone, listed leader task", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task + "7/stat": statLine(7, "Z", 7, "1", 900)}, nil, gone},
		{"zombie alone, listed task vanished", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task + "8/x": ""},
			map[string]error{task + "8/stat": fs.ErrNotExist}, gone},
		{"zombie alone, task stat EACCES", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task + "8/x": ""},
			map[string]error{task + "8/stat": syscall.EACCES}, unknown},
		{"zombie alone, task stat EIO", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task + "8/x": ""},
			map[string]error{task + "8/stat": syscall.EIO}, unknown},
		{"zombie alone, task stat malformed", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task + "8/stat": "8 (a"}, nil, unknown},
		// A running task the count did not include: live only while the
		// leader is still the same incarnation.
		{"uncounted running task, same leader", map[string]string{leader: statLine(7, "Z", 7, "1", 900), task + "8/stat": statLine(8, "R", 7, "1", 900)}, nil, live},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := map[string]string{testBoot: "b\n"}
			for k, v := range c.files {
				files[k] = v
			}
			start, err := fakeProc(files, c.errs).startTime(7)
			got := live
			switch {
			case errors.Is(err, errProcessGone):
				got = gone
			case err != nil:
				got = unknown
			case start != "b:900":
				t.Fatalf("start %q", start)
			}
			if got != c.want {
				t.Fatalf("got %s (%v), want %s", got, err, c.want)
			}
		})
	}
}

// The incarnation re-read after an uncounted running task: a changed or
// vanished leader proves the recorded one gone; an unreadable one is unknown.
func TestProcStartTimeRevalidatesLeaderIncarnation(t *testing.T) {
	for name, c := range map[string]struct {
		again   string
		err     error
		gone    bool
		unknown bool
	}{
		"recycled pid":   {again: statLine(7, "S", 7, "1", 901), gone: true},
		"leader reaped":  {err: fs.ErrNotExist, gone: true},
		"re-read EACCES": {err: syscall.EACCES, unknown: true},
	} {
		t.Run(name, func(t *testing.T) {
			reads := 0
			p := fakeProc(map[string]string{testBoot: "b", "/proc/7/task/8/stat": statLine(8, "R", 7, "1", 905)}, nil)
			base := p.readFile
			p.readFile = func(path string) ([]byte, error) {
				if path != "/proc/7/stat" {
					return base(path)
				}
				if reads++; reads == 1 {
					return []byte(statLine(7, "Z", 7, "1", 900)), nil
				}
				return []byte(c.again), c.err
			}
			_, err := p.startTime(7)
			if gone := errors.Is(err, errProcessGone); gone != c.gone || (err != nil && !gone) != c.unknown {
				t.Fatalf("err %v, want gone=%v unknown=%v", err, c.gone, c.unknown)
			}
		})
	}
}

// Full-width records still need valid numeric identity fields. Otherwise a
// malformed starttime can masquerade as a new incarnation, or a malformed
// pgrp can silently remove an unreadable process from the captured group.
func TestProcStatRejectsMalformedIdentityFields(t *testing.T) {
	for _, field := range []struct {
		index int
		name  string
	}{{2, "pgrp"}, {19, "starttime"}} {
		for _, value := range []string{"oops", "-1", "18446744073709551616"} {
			t.Run(field.name+"/"+value, func(t *testing.T) {
				line := statLine(7, "S", 7, "1", 900)
				end := strings.LastIndexByte(line, ')')
				fields := strings.Fields(line[end+1:])
				fields[field.index] = value
				line = line[:end+1] + " " + strings.Join(fields, " ")
				p := fakeProc(map[string]string{testBoot: "b", "/proc/7/stat": line}, nil)
				if _, err := p.startTime(7); err == nil || errors.Is(err, errProcessGone) {
					t.Fatalf("malformed %s %q returned %v, want unknown", field.name, value, err)
				}
				if members, err := p.groupIdentities(7); err == nil {
					t.Fatalf("malformed %s captured %v without error", field.name, members)
				}
			})
		}
	}
}

// Group capture skips proven-gone members and fails, rather than drops, a
// member whose liveness is unknown.
func TestProcGroupIdentitiesFailsOnUnknownMember(t *testing.T) {
	files := map[string]string{
		testBoot:        "b",
		"/proc/7/stat":  statLine(7, "S", 7, "2", 900),
		"/proc/8/stat":  statLine(8, "Z", 7, "3", 901), // leader exited, threads remain
		"/proc/9/stat":  statLine(9, "Z", 7, "1", 902), // whole group done
		"/proc/10/x":    "",                            // vanished while listed
		"/proc/11/stat": statLine(11, "S", 5, "1", 903),
		"/proc/self/x":  "",
	}
	p := fakeProc(files, map[string]error{"/proc/10/stat": fs.ErrNotExist, "/proc/9/task": fs.ErrNotExist})
	members, err := p.groupIdentities(7)
	want := []processIdentity{{7, "b:900"}, {8, "b:901"}}
	if err != nil || fmt.Sprint(members) != fmt.Sprint(want) {
		t.Fatalf("members %v err %v, want %v", members, err, want)
	}
	for _, fault := range []map[string]error{
		{"/proc/10/stat": syscall.EACCES},
		{"/proc/10/stat": fs.ErrNotExist, "/proc/9/task": syscall.EIO},
	} {
		if members, err := fakeProc(files, fault).groupIdentities(7); err == nil {
			t.Fatalf("fault %v: captured %v without error", fault, members)
		}
	}
	files["/proc/8/stat"] = statLine(8, "Z", 7, "?", 901)
	if members, err := fakeProc(files, nil).groupIdentities(7); err == nil {
		t.Fatalf("malformed num_threads captured %v without error", members)
	}
}

// Unknown liveness is never completion: reclaim keeps the ownership record
// whether the orphan or its monitor cannot be read, and a captured member
// that cannot be read is still waited for.
func TestReclaimKeepsOwnershipWhenLivenessUnknown(t *testing.T) {
	saved := startTimeOf
	t.Cleanup(func() { startTimeOf = saved })
	unreadable := map[int]bool{}
	startTimeOf = func(pid int) (string, error) {
		if unreadable[pid] {
			return "", fmt.Errorf("read /proc/%d/stat: %w", pid, syscall.EIO)
		}
		return "", fmt.Errorf("%w: test", errProcessGone)
	}
	root := t.TempDir()
	orphan := writeOwner(t, root, "orphan", unitOwner{PID: 1<<30 + 1, Start: "s", Monitor: 1<<30 + 2, MonitorStart: "m"})
	unreadable[1<<30+1] = true
	if lines := reclaimOrphans(root); len(lines) != 1 || !strings.Contains(lines[0], "liveness unknown") {
		t.Fatalf("unknown orphan: %q", lines)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("an orphan of unknown liveness lost its ownership record")
	}
	unreadable = map[int]bool{1<<30 + 2: true}
	if lines := reclaimOrphans(root); len(lines) != 1 || !strings.Contains(lines[0], "monitor pid") {
		t.Fatalf("unknown monitor: %q", lines)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("an orphan of an unknown monitor lost its ownership record")
	}
	unreadable = map[int]bool{}
	if lines := reclaimOrphans(root); len(lines) != 0 {
		t.Fatalf("proven-gone orphan: %q", lines)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("a proven-gone orphan's record should be dropped")
	}

	unreadable = map[int]bool{1<<30 + 3: true}
	if waitProcessesGone([]processIdentity{{1<<30 + 3, "s"}}, 50*time.Millisecond) {
		t.Fatal("a member of unknown liveness counted as gone")
	}
	if !waitProcessesGone([]processIdentity{{1<<30 + 4, "s"}}, 50*time.Millisecond) {
		t.Fatal("a proven-gone member was waited for")
	}
}
