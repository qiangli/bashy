// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Command clip is the clip tool (S8): an asynchronous clip library on the
// media family. queue returns at once with a job id; work renders queued
// jobs one at a time (ffmpeg for video, agg for terminal scenes, X11 for
// the screen); take prints a finished clip's path so it composes with the
// video and vlog examples. serve is the loopback library page.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const usage = `clip — an asynchronous clip library

  clip queue --from VIDEO.mp4 --start 12 --end 18 [--name NAME] [--data DIR]
  clip queue --from SCENE.cast [--format gif|mp4] [--start S --end E]
  clip queue screen [--seconds N] [--display :0.0] [--size WxH]
  clip status [ID] [--json]        job list, or one job
  clip work [--once]               render queued jobs (one with --once)
  clip take ID                     print a finished clip's path
                                     exit 0 done, 3 still waiting, 1 failed
  clip serve [-addr HOST:PORT]     the library page (loopback)
  clip doctor                      check for ffmpeg and agg

CLIP_DATA selects the data directory (default $BASHY_APP_DATA when set, else
a "clip" directory under the Bashy home). --json prints machine-readable
records; queue always prints the job id first.`

// exitPending mirrors the family's meaningful status: nothing wrong, but
// the clip is not ready.
const exitPending = 3

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "clip:", err)
		os.Exit(1)
	}
}

func dataDir() (string, error) {
	if d := os.Getenv("CLIP_DATA"); d != "" {
		return filepath.Abs(d)
	}
	if d := os.Getenv("BASHY_APP_DATA"); d != "" {
		return filepath.Abs(d)
	}
	home := os.Getenv("BASHY_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no data directory: set CLIP_DATA")
		}
		home = h + "/.bashy"
	}
	return filepath.Abs(filepath.Join(home, "desk", "clip"))
}

// reorderArgs puts flag arguments (with their values) before positionals,
// so `clip status c123 --json` and `clip status --json c123` both parse
// with the standard flag package.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags, pos []string
	restArePos := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if restArePos || a == "--" {
			if a == "--" && !restArePos {
				restArePos = true
				continue
			}
			pos = append(pos, a)
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(a, "--"), "-")
		if strings.HasPrefix(a, "-") && name != "" {
			hasValue := strings.Contains(a, "=")
			boolFlag := false
			if !hasValue {
				if f := fs.Lookup(name); f != nil {
					if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
						boolFlag = true
					}
				}
			}
			flags = append(flags, a)
			if !hasValue && !boolFlag && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}

func run(args []string) error {
	verb := args[0]
	args = args[1:]
	switch verb {
	case "-h", "--help", "help":
		fmt.Println(usage)
		return nil
	case "queue":
		return cmdQueue(args)
	case "status", "list":
		return cmdStatus(args)
	case "work":
		return cmdWork(args)
	case "take":
		return cmdTake(args)
	case "serve":
		return cmdServe(args)
	case "doctor":
		return cmdDoctor(args)
	default:
		return fmt.Errorf("unknown verb %q\n\n%s", verb, usage)
	}
}

// queueFlags is the shared queue surface: the CLI and the page post the
// same spec.
type queueFlags struct {
	from, name, format, display, size string
	start, end, seconds               float64
	jsonOut                           bool
	data                              string
}

func addQueueFlags(fs *flag.FlagSet, q *queueFlags) {
	fs.StringVar(&q.from, "from", "", "source file: a video to cut, or an asciicast v2 scene")
	fs.StringVar(&q.name, "name", "", "display name for the clip")
	fs.StringVar(&q.format, "format", "", "cast scenes: gif (default) or mp4")
	fs.StringVar(&q.display, "display", "", "screen: X11 display (default :0.0)")
	fs.StringVar(&q.size, "size", "", "screen: capture size WxH (default 1280x720)")
	fs.Float64Var(&q.start, "start", 0, "cut start, seconds")
	fs.Float64Var(&q.end, "end", 0, "cut end, seconds")
	fs.Float64Var(&q.seconds, "seconds", 0, "screen: how long to record")
	fs.BoolVar(&q.jsonOut, "json", false, "print the job as JSON")
	fs.StringVar(&q.data, "data", "", "data directory")
}

func (q *queueFlags) spec() *Job {
	j := &Job{Kind: KindTrim, Name: q.name, Format: q.format, Display: q.display, Size: q.size,
		Start: q.start, End: q.end, Seconds: q.seconds, Source: q.from}
	if q.from == "" {
		j.Kind = KindScreen
	} else if filepath.Ext(q.from) == ".cast" {
		j.Kind = KindCast
	}
	return j
}

func cmdQueue(args []string) error {
	var q queueFlags
	fs := flag.NewFlagSet("queue", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(fs.Output(), usage) }
	addQueueFlags(fs, &q)
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() > 0 && fs.Arg(0) == "screen" {
		q.from = "" // `queue screen` is the explicit form of a screen job
	}
	dir := q.data
	if dir == "" {
		var err error
		dir, err = dataDir()
		if err != nil {
			return err
		}
	}
	s, err := NewStore(dir)
	if err != nil {
		return err
	}
	j, err := s.Queue(q.spec())
	if err != nil {
		return err
	}
	if q.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(j)
	}
	fmt.Println(j.ID)
	fmt.Printf("queued: %s clip %s (%s)\n", j.Kind, j.ID, spanText(j))
	fmt.Printf("render it with: clip work --data %s\n", s.Dir())
	return nil
}

func spanText(j *Job) string {
	if j.Kind == KindScreen {
		return trimZero(j.Seconds) + "s"
	}
	return trimZero(j.Start) + "-" + trimZero(j.End) + "s"
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	var data, id string
	var jsonOut bool
	fs.StringVar(&data, "data", "", "data directory")
	fs.BoolVar(&jsonOut, "json", false, "print jobs as JSON")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), usage) }
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		id = fs.Arg(0)
	}
	dir := data
	if dir == "" {
		var err error
		dir, err = dataDir()
		if err != nil {
			return err
		}
	}
	s, err := NewStore(dir)
	if err != nil {
		return err
	}
	if id != "" {
		j, err := s.Load(id)
		if err != nil {
			return err
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(j)
		}
		printJob(j)
		return nil
	}
	jobs, err := s.List()
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}
	if len(jobs) == 0 {
		fmt.Println("no clips yet; queue one with: clip queue --from VIDEO.mp4 --start S --end E")
		return nil
	}
	for _, j := range jobs {
		printJob(j)
	}
	return nil
}

func printJob(j *Job) {
	line := fmt.Sprintf("%s  %-8s %-7s %s", j.ID, j.Kind, j.Status, spanText(j))
	if j.Name != "" {
		line += "  " + j.Name
	}
	fmt.Println(line)
	if j.Error != "" {
		fmt.Println("    " + j.Error)
	}
}

func cmdWork(args []string) error {
	fs := flag.NewFlagSet("work", flag.ContinueOnError)
	var data string
	var once, verbose bool
	fs.StringVar(&data, "data", "", "data directory")
	fs.BoolVar(&once, "once", false, "render exactly one queued job")
	fs.BoolVar(&verbose, "verbose", false, "print each rendered job")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), usage) }
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	dir := data
	if dir == "" {
		var err error
		dir, err = dataDir()
		if err != nil {
			return err
		}
	}
	s, err := NewStore(dir)
	if err != nil {
		return err
	}
	n, err := Work(context.Background(), s, ExecRunner{}, once)
	if verbose && n > 0 {
		fmt.Printf("rendered %d clip(s)\n", n)
	}
	return err
}

func cmdTake(args []string) error {
	fs := flag.NewFlagSet("take", flag.ContinueOnError)
	var data string
	fs.StringVar(&data, "data", "", "data directory")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), usage) }
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("take needs one job id")
	}
	dir := data
	if dir == "" {
		var err error
		dir, err = dataDir()
		if err != nil {
			return err
		}
	}
	s, err := NewStore(dir)
	if err != nil {
		return err
	}
	j, err := s.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	switch j.Status {
	case StatusDone:
		p, err := ArtifactPath(s, j)
		if err != nil {
			return err
		}
		fmt.Println(p)
		return nil
	case StatusQueued, StatusRunning:
		fmt.Fprintf(os.Stderr, "clip: %s is %s\n", j.ID, j.Status)
		os.Exit(exitPending)
		return nil
	default:
		return fmt.Errorf("%s failed: %s", j.ID, j.Error)
	}
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	var addr, data string
	fs.StringVar(&addr, "addr", envDefault("CLIP_ADDR", "127.0.0.1:22807"), "listen address")
	fs.StringVar(&data, "data", "", "data directory")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), usage) }
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if data == "" {
		var err error
		data, err = dataDir()
		if err != nil {
			return err
		}
	}
	s, err := NewStore(data)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Printf("clips: http://%s/  (library: %s)\n", ln.Addr(), s.Dir())
	srv := &http.Server{Handler: Serve(s), ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func cmdDoctor(args []string) error {
	for _, tool := range []string{"ffmpeg", "agg"} {
		if path, err := exec.LookPath(tool); err != nil {
			fmt.Printf("%s: missing (%s)\n", tool, why(tool))
		} else {
			fmt.Printf("%s: %s\n", tool, path)
		}
	}
	return nil
}

func why(tool string) string {
	if tool == "agg" {
		return "renders terminal scenes; video clips still work without it"
	}
	return "needed for video and screen clips"
}
