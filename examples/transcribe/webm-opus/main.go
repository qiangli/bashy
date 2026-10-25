// A standalone ordinary stage-v1 adapter, not a core dispatch extension.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const version = "bashy-transcribe-stage-v1"

type artifact struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}
type request struct {
	Version   string         `json:"version"`
	Stage     string         `json:"stage"`
	WorkDir   string         `json:"work_dir"`
	Artifacts []artifact     `json:"artifacts"`
	Media     map[string]any `json:"media,omitempty"`
}
type stageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type result struct {
	Version   string         `json:"version"`
	Stage     string         `json:"stage"`
	Artifacts []artifact     `json:"artifacts,omitempty"`
	Media     map[string]any `json:"media,omitempty"`
	Error     *stageError    `json:"error,omitempty"`
}

func boundedRead(ctx context.Context, r io.Reader, limit int) ([]byte, error) {
	var out []byte
	buf := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		if n > limit-len(out) {
			return nil, fmt.Errorf("input size limit")
		}
		out = append(out, buf[:n]...)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}
func regular(path string) (*os.File, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be a regular file, not a symlink/device/pipe")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(st, actual) {
		f.Close()
		return nil, fmt.Errorf("input changed while opening")
	}
	return f, nil
}
func execute(ctx context.Context, in request) (res result, status int) {
	res = result{Version: version, Stage: "decode"}
	status = 1
	fail := func(err error) (result, int) {
		code := "invalid_media"
		s := 1
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			code = "cancelled"
			s = 130
		}
		return result{Version: version, Stage: "decode", Error: &stageError{Code: code, Message: err.Error()}}, s
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if in.Version != version || in.Stage != "decode" || len(in.Artifacts) != 1 || !filepath.IsAbs(in.WorkDir) {
		return result{Version: version, Stage: "decode", Error: &stageError{Code: "invalid_request", Message: "decode requires stage v1, exactly one artifact and absolute work_dir"}}, 2
	}
	root, err := filepath.EvalSymlinks(in.WorkDir)
	if err != nil {
		return fail(err)
	}
	st, err := os.Stat(root)
	if err != nil {
		return fail(err)
	}
	if !st.IsDir() {
		return fail(fmt.Errorf("work_dir must be a directory"))
	}
	src, err := regular(in.Artifacts[0].Path)
	if err != nil {
		return fail(err)
	}
	defer src.Close()
	st, err = src.Stat()
	if err != nil {
		return fail(err)
	}
	if st.Size() > maxInput {
		return fail(fmt.Errorf("input size limit"))
	}
	data, err := boundedRead(ctx, src, maxInput)
	if err != nil {
		return fail(err)
	}
	if want := in.Artifacts[0].SHA256; want != "" {
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != want {
			return fail(fmt.Errorf("input SHA-256 mismatch"))
		}
	}
	m, err := demux(ctx, data)
	if err != nil {
		return fail(err)
	}
	owned, err := os.MkdirTemp(root, ".webm-opus-")
	if err != nil {
		return fail(err)
	}
	published := false
	defer func() {
		if !published {
			os.RemoveAll(owned)
		}
	}()
	tmp := filepath.Join(owned, "audio.partial")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fail(err)
	}
	defer f.Close()
	if _, err = f.Write(make([]byte, 44)); err != nil {
		return fail(err)
	}
	frames, err := decode(ctx, m, f)
	if err != nil {
		return fail(err)
	}
	if _, err = f.WriteAt(wavHeader(frames), 0); err != nil {
		return fail(err)
	}
	if err = f.Sync(); err != nil {
		return fail(err)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	hash := sha256.New()
	buf := make([]byte, 64<<10)
	for {
		if err = ctx.Err(); err != nil {
			return fail(err)
		}
		n, e := f.Read(buf)
		hash.Write(buf[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			return fail(e)
		}
	}
	if err = f.Close(); err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	dest := filepath.Join(owned, "audio.wav")
	if err = os.Rename(tmp, dest); err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	meta := map[string]any{}
	for k, v := range in.Media {
		meta[k] = v
	}
	meta["sample_rate_hz"] = 16000
	meta["channels"] = 1
	meta["duration_ms"] = frames * 1000 / 16000
	res.Artifacts = []artifact{{Path: dest, MediaType: "audio/wav", SHA256: hex.EncodeToString(hash.Sum(nil))}}
	res.Media = meta
	published = true
	return res, 0
}
func run(ctx context.Context, args []string, out io.Writer) int {
	bad := func(err error) int {
		json.NewEncoder(out).Encode(result{Version: version, Stage: "decode", Error: &stageError{Code: "invalid_request", Message: err.Error()}})
		return 2
	}
	if ctx.Err() != nil {
		res, s := execute(ctx, request{})
		json.NewEncoder(out).Encode(res)
		return s
	}
	if len(args) != 1 {
		return bad(fmt.Errorf("usage: transcribe-webm-opus REQUEST.json"))
	}
	f, err := regular(args[0])
	if err != nil {
		return bad(err)
	}
	defer f.Close()
	data, err := boundedRead(ctx, f, 1<<20)
	if err != nil {
		if ctx.Err() != nil {
			res, s := execute(ctx, request{})
			json.NewEncoder(out).Encode(res)
			return s
		}
		return bad(err)
	}
	var in request
	if err = json.Unmarshal(data, &in); err != nil {
		return bad(err)
	}
	res, status := execute(ctx, in)
	if err = json.NewEncoder(out).Encode(res); err != nil {
		if status == 0 {
			os.RemoveAll(filepath.Dir(res.Artifacts[0].Path))
		}
		return 1
	}
	return status
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout))
}
