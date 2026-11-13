// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Runner runs one external tool. The real one spawns a child; tests inject
// a recording fake. Output is bounded so a loud tool cannot flood the job
// record.
type Runner interface {
	Run(ctx context.Context, tool string, args ...string) (string, error)
}

// ExecRunner runs tools as bounded children: no shell, output capped.
type ExecRunner struct{}

// logLimit bounds the worker output kept in the job record.
const logLimit = 8 << 10

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, tool string, args ...string) (string, error) {
	path, err := exec.LookPath(tool)
	if err != nil {
		return "", fmt.Errorf("%s is not installed", tool)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	log := out.String()
	if len(log) > logLimit {
		log = log[len(log)-logLimit:]
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return log, fmt.Errorf("%s exited %d", tool, ee.ExitCode())
		}
		return log, err
	}
	return log, nil
}

// staleAfter is how long a job may sit in running before a dead worker is
// presumed and the job returns to the queue.
const staleAfter = 10 * time.Minute

// Work processes queued jobs one at a time until none is waiting (or --once:
// exactly one). A failed job is recorded and does not stop the queue. It
// returns how many jobs were processed.
func Work(ctx context.Context, s *Store, r Runner, once bool) (int, error) {
	if err := requeueStale(s); err != nil {
		return 0, err
	}
	n := 0
	for {
		if once && n >= 1 {
			return n, nil
		}
		j, err := s.Next()
		if err != nil {
			return n, err
		}
		if j == nil {
			return n, nil
		}
		if err := process(ctx, s, r, j); err != nil {
			return n, err
		}
		n++
	}
}

// requeueStale returns running jobs from a worker that died mid-render to
// the queue. Artifacts are written by rename, so re-running is safe.
func requeueStale(s *Store) error {
	jobs, err := s.List()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Status != StatusRunning {
			continue
		}
		up, err := time.Parse(time.RFC3339Nano, j.Updated)
		if err != nil || time.Since(up) > staleAfter {
			j.Status = StatusQueued
			j.Error = ""
			if err := s.Save(j); err != nil {
				return err
			}
		}
	}
	return nil
}

// process renders one job through to done or failed.
func process(ctx context.Context, s *Store, r Runner, j *Job) error {
	j.Status = StatusRunning
	j.Error, j.Log = "", ""
	if err := s.Save(j); err != nil {
		return err
	}
	var (
		log string
		err error
	)
	switch j.Kind {
	case KindTrim:
		log, err = runTrim(ctx, r, j, s.ClipsDir())
	case KindCast:
		log, err = runCast(ctx, r, j, s.ClipsDir())
	case KindScreen:
		log, err = runScreen(ctx, r, j, s.ClipsDir())
	default:
		err = fmt.Errorf("clips: unknown kind %q", j.Kind)
	}
	j.Log = log
	if err != nil {
		j.Status = StatusFailed
		j.Error = err.Error()
	} else {
		j.Status = StatusDone
	}
	return s.Save(j)
}

func secs(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// runTrim cuts [start,end) out of a video with ffmpeg, re-encoding so the
// cut lands exactly where asked.
func runTrim(ctx context.Context, r Runner, j *Job, clipsDir string) (string, error) {
	out := filepath.Join(clipsDir, j.ID+".mp4")
	args := []string{"-nostdin", "-loglevel", "error", "-ss", secs(j.Start), "-to", secs(j.End), "-i", j.Source,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-c:a", "aac", "-movflags", "+faststart", out}
	log, err := r.Run(ctx, "ffmpeg", args...)
	if err != nil {
		_ = os.Remove(out) // a partial encode is not a clip
		return log, err
	}
	j.Artifact = "clips/" + j.ID + ".mp4"
	return log, nil
}

// runCast renders an asciicast v2 scene with agg to a looping gif, and to
// mp4 through ffmpeg when asked.
func runCast(ctx context.Context, r Runner, j *Job, clipsDir string) (string, error) {
	gif := filepath.Join(clipsDir, j.ID+".gif")
	log, err := r.Run(ctx, "agg", j.Source, gif)
	if err != nil {
		return log, err
	}
	if j.Format != "mp4" {
		j.Artifact = "clips/" + j.ID + ".gif"
		return log, nil
	}
	mp4 := filepath.Join(clipsDir, j.ID+".mp4")
	log2, err := r.Run(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-i", gif,
		"-movflags", "+faststart", "-pix_fmt", "yuv420p", mp4)
	log = log + log2
	if err != nil {
		_ = os.Remove(gif) // the mp4 is the artifact; a lone gif is not
		_ = os.Remove(mp4)
		return log, err
	}
	_ = os.Remove(gif)
	j.Artifact = "clips/" + j.ID + ".mp4"
	return log, nil
}

// runScreen records the screen. Screen capture is an X11 job: this sprint
// targets Linux; elsewhere the job fails with the reason, never a hang.
func runScreen(ctx context.Context, r Runner, j *Job, clipsDir string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("screen clips need Linux (X11); this host runs " + runtime.GOOS)
	}
	size := j.Size
	if size == "" {
		size = "1280x720"
	}
	display := j.Display
	if display == "" {
		display = ":0.0"
	}
	out := filepath.Join(clipsDir, j.ID+".mp4")
	args := []string{"-nostdin", "-loglevel", "error", "-f", "x11grab", "-framerate", "15",
		"-video_size", size, "-i", display, "-t", secs(j.Seconds),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p", out}
	log, err := r.Run(ctx, "ffmpeg", args...)
	if err != nil {
		_ = os.Remove(out)
		return log, err
	}
	j.Artifact = "clips/" + j.ID + ".mp4"
	return log, nil
}

// ArtifactPath resolves a job's artifact under the store, refusing anything
// that escapes the clips directory.
func ArtifactPath(s *Store, j *Job) (string, error) {
	if j.Artifact == "" {
		return "", errors.New("clips: job has no artifact")
	}
	clean := filepath.Clean(j.Artifact)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("clips: artifact escapes the library")
	}
	return filepath.Join(s.Dir(), clean), nil
}
