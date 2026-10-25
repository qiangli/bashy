// This helper is registered like any user adapter; it has no core dispatch hook.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	c "github.com/qiangli/bashy/internal/transcribecontract"
	"github.com/qiangli/bashy/internal/transcribeseed"
)

func main() { os.Exit(mainStatus()) }
func mainStatus() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fs := flag.NewFlagSet("transcribe-adapter", flag.ContinueOnError)
	var cfg transcribeseed.Config
	fs.StringVar(&cfg.WhisperPath, "whisper", "", "admitted whisper.cpp executable path")
	fs.StringVar(&cfg.WhisperSHA256, "whisper-sha256", "", "reviewed executable SHA-256")
	fs.StringVar(&cfg.FFmpegPath, "ffmpeg", "", "policy-admitted decoder executable path")
	fs.StringVar(&cfg.FFmpegSHA256, "ffmpeg-sha256", "", "reviewed executable SHA-256")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: transcribe-adapter [deployment flags] local|wav|ffmpeg|whisper|export REQUEST.json; or model MODEL")
		return 2
	}
	if fs.Arg(0) == "model" {
		path, err := transcribeseed.SetupModel(ctx, fs.Arg(1))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintln(os.Stdout, path)
		return 0
	}
	f, err := os.Open(fs.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer f.Close()
	in, err := transcribeseed.ReadRequest(f)
	if err != nil {
		json.NewEncoder(os.Stdout).Encode(c.TranscribeStageResult{Version: c.TranscribeStageVersion, Stage: in.Stage, Error: &c.TranscribeStageError{Code: "invalid_request", Message: err.Error()}})
		return 2
	}
	out, status := transcribeseed.Execute(ctx, fs.Arg(0), in, cfg)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return status
}
