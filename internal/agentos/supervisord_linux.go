// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// reaper collects the zombies of adopted orphans when the supervisor is the
// one they get reparented to: PID 1 in a container, or a child subreaper
// (prctl PR_SET_CHILD_SUBREAPER), which the supervisor becomes whenever it can
// so the contract is the same in and out of a container. Left unreaped, those
// zombies would pile up for the life of the supervisor (u-root's
// libinit.WaitOrphans and ochinchina/supervisord's ReapZombie exist for this).
//
// The race that matters: the reaper's wait4(-1) can collect the MAIN child
// before os/exec's Wait does, and then Wait fails with ECHILD and the exit
// status — the one fact restart policy depends on — is lost. Rather than
// peeking at siginfo layouts, the reaper keeps what it took: arm() before a
// spawn empties the stash and stashes everything reaped until watch(pid) names
// the child (the spawn window is microseconds and a stale pid cannot be the
// live child's), after which only that pid is kept; execChild.Wait consults
// take(pid) when its own wait comes back empty. Orphans are reaped and
// counted, never stored.
type reaper struct {
	mu     sync.Mutex
	main   int // 0 = spawning: stash every pid
	stash  map[int]childExit
	orphan int // adopted orphans reaped (a test's only observable)

	chld chan os.Signal
	done chan struct{}
}

// startReaper turns the reaper on when this process is PID 1 or can become a
// subreaper; nil otherwise (then nobody's orphans are ours).
func startReaper() *reaper {
	if os.Getpid() != 1 {
		if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
			return nil
		}
	}
	return newReaper()
}

func newReaper() *reaper {
	r := &reaper{stash: map[int]childExit{}, chld: make(chan os.Signal, 1), done: make(chan struct{})}
	signal.Notify(r.chld, syscall.SIGCHLD)
	go r.loop()
	return r
}

func (r *reaper) loop() {
	defer close(r.done)
	for range r.chld {
		r.reap()
	}
}

// reap collects every zombie that is ready, without blocking (WNOHANG), until
// there are none: SIGCHLD coalesces, so one wake may owe several waits.
func (r *reaper) reap() {
	for {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || pid <= 0 {
			return
		}
		r.mu.Lock()
		switch {
		case r.main == 0 || pid == r.main:
			code, signaled := 0, false
			if ws.Signaled() {
				code, signaled = 128+int(ws.Signal()), true
			} else {
				code = ws.ExitStatus()
			}
			r.stash[pid] = childExit{code: code, signaled: signaled}
		default:
			r.orphan++
		}
		r.mu.Unlock()
	}
}

// arm is called before a spawn: nothing stashed can belong to the next child.
func (r *reaper) arm() {
	r.mu.Lock()
	r.main = 0
	r.stash = map[int]childExit{}
	r.mu.Unlock()
}

// watch names the live child; everything stashed for another pid during the
// spawn window was an orphan.
func (r *reaper) watch(pid int) {
	r.mu.Lock()
	r.main = pid
	for p := range r.stash {
		if p != pid {
			delete(r.stash, p)
			r.orphan++
		}
	}
	r.mu.Unlock()
}

// take hands back the status of the main child if the reaper collected it.
func (r *reaper) take(pid int) (childExit, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ex, ok := r.stash[pid]
	delete(r.stash, pid)
	return ex, ok
}

func (r *reaper) orphansReaped() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.orphan
}

func (r *reaper) stop() {
	signal.Stop(r.chld)
	close(r.chld)
	<-r.done
}
