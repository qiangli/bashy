// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// leaderExitListener stays reachable from a package variable, so a garbage
// collection after the main thread exits cannot finalize and close it.
var leaderExitListener net.Listener

// The leader-thread-exit child runs from init, which Go runs on the main
// thread: it binds the port, announces, then exits only that thread. The
// kernel then reports the group leader as a zombie while the runtime's other
// threads keep the shared descriptor table — and the listener — alive.
func init() {
	arg, ok := strings.CutPrefix(os.Getenv("BASHY_SUPERVISORD_TEST_CHILD"), "leader-thread-exit-listen:")
	if !ok {
		return
	}
	port, ready, _ := strings.Cut(arg, ",")
	var err error
	if leaderExitListener, err = net.Listen("tcp", "127.0.0.1:"+port); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(ready, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(3)
	}
	syscall.RawSyscall(syscall.SYS_EXIT, 0, 0, 0)
}

// A process whose leader thread exited still owns its listener until every
// thread has: it is live for identity, group capture and reclaim completion.
func TestReclaimOrphansWaitsForThreadsOfExitedLeader(t *testing.T) {
	root, port := t.TempDir(), freePort(t)
	ready := filepath.Join(root, "ready")
	c := exec.Command(os.Args[0], "leader-thread-exit-listen")
	c.Env = append(os.Environ(), "BASHY_SUPERVISORD_TEST_CHILD=leader-thread-exit-listen:"+port+","+ready)
	c.SysProcAttr = ownProcessGroupAttr()
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	pid := c.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL); _ = c.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := procStatFields("/proc/" + strconv.Itoa(pid) + "/stat")
		if _, rerr := os.Stat(ready); rerr == nil && err == nil && f[0] == "Z" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("leader thread never exited with the listener held: %s", portDiagnosis(port, pid))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := checkPorts(listenerUnit(port)); !errors.Is(err, errForeignListener) {
		t.Fatalf("precondition listener: %v", err)
	}
	start, err := procStartTime(pid)
	if err != nil {
		t.Fatalf("live thread group with an exited leader reported gone: %v", err)
	}
	members, err := processGroupIdentities(pid)
	if err != nil || len(members) != 1 || members[0] != (processIdentity{pid, start}) {
		t.Fatalf("group capture %v, error %v", members, err)
	}
	writeOwner(t, root, "listener", unitOwner{PID: pid, Start: start, Monitor: 1 << 30, MonitorStart: "gone", Signal: int(syscall.SIGTERM), GraceMS: 100})
	lines := reclaimOrphans(root)
	if len(lines) != 1 || sameProcess(pid, start) {
		t.Fatalf("reclaim returned before the thread group exited: %q", lines)
	}
	if err := checkPorts(listenerUnit(port)); err != nil {
		t.Fatalf("reclaimed group still holds port: %v; %s", err, portDiagnosis(port, pid))
	}
}

// portDiagnosis separates an accepting listener from a refused bind, lists
// the kernel's sockets on port by TCP state, and names each pid's tasks.
func portDiagnosis(port string, pids ...int) string {
	var b strings.Builder
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 250*time.Millisecond); err == nil {
		c.Close()
		b.WriteString("dial=accepted")
	} else {
		fmt.Fprintf(&b, "dial=%v", err)
	}
	if l, err := net.Listen("tcp", "127.0.0.1:"+port); err == nil {
		l.Close()
		b.WriteString(" bind=ok")
	} else {
		fmt.Fprintf(&b, " bind=%v", err)
	}
	n, _ := strconv.Atoi(port)
	hex := fmt.Sprintf(":%04X", n)
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, _ := os.ReadFile(table)
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			// local_address rem_address st (01 ESTABLISHED, 06 TIME_WAIT, 0A LISTEN, …) … inode
			if len(f) > 9 && (strings.HasSuffix(f[1], hex) || strings.HasSuffix(f[2], hex)) {
				fmt.Fprintf(&b, " %s[%s->%s st=%s inode=%s]", filepath.Base(table), f[1], f[2], f[3], f[9])
			}
		}
	}
	for _, pid := range pids {
		dir := "/proc/" + strconv.Itoa(pid) + "/task"
		tasks, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(&b, " pid%d=%v", pid, err)
			continue
		}
		fmt.Fprintf(&b, " pid%d tasks:", pid)
		for _, task := range tasks {
			state := "?"
			if f, err := procStatFields(dir + "/" + task.Name() + "/stat"); err == nil {
				state = f[0]
			}
			fmt.Fprintf(&b, " %s=%s", task.Name(), state)
		}
	}
	return b.String()
}
