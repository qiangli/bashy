package transcribeseed

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	c "github.com/qiangli/bashy/internal/transcribecontract"
)

// WAV is the dependency-free seed: finite RIFF PCM16, any channel count up to
// eight and sample rates up to 192 kHz. Other encodings use another decoder.
// Parsing is bounded by the declared RIFF and actual file sizes, not chunk
// allocations. Normalization streams with constant memory.
type wavInfo struct {
	rate, channels int
	offset, size   int64
}

func readWAV(f *os.File) (wavInfo, error) {
	var w wavInfo
	fi, err := f.Stat()
	if err != nil {
		return w, err
	}
	var h [12]byte
	if _, err = io.ReadFull(f, h[:]); err != nil {
		return w, err
	}
	end := int64(binary.LittleEndian.Uint32(h[4:8])) + 8
	if string(h[:4]) != "RIFF" || string(h[8:]) != "WAVE" || end > fi.Size() || end < 12 {
		return w, fmt.Errorf("invalid RIFF/WAVE file")
	}
	format, data := false, false
	for pos := int64(12); pos < end; {
		var ch [8]byte
		if end-pos < 8 {
			return w, fmt.Errorf("truncated WAV chunk")
		}
		if _, err = f.ReadAt(ch[:], pos); err != nil {
			return w, err
		}
		n := int64(binary.LittleEndian.Uint32(ch[4:]))
		start := pos + 8
		if n > end-start || n+(n%2) > end-start {
			return w, fmt.Errorf("truncated WAV chunk payload")
		}
		switch string(ch[:4]) {
		case "fmt ":
			if format || n < 16 {
				return w, fmt.Errorf("invalid WAV format chunk")
			}
			var b [16]byte
			if _, err = f.ReadAt(b[:], start); err != nil {
				return w, err
			}
			w.channels = int(binary.LittleEndian.Uint16(b[2:4]))
			w.rate = int(binary.LittleEndian.Uint32(b[4:8]))
			if binary.LittleEndian.Uint16(b[:2]) != 1 || binary.LittleEndian.Uint16(b[14:]) != 16 || w.channels < 1 || w.channels > 8 || w.rate < 1000 || w.rate > 192000 || int(binary.LittleEndian.Uint16(b[12:14])) != w.channels*2 || int(binary.LittleEndian.Uint32(b[8:12])) != w.rate*w.channels*2 {
				return w, fmt.Errorf("WAV seed requires PCM16, 1–8 channels, 1–192 kHz; select another decoder for this encoding")
			}
			format = true
		case "data":
			if data {
				return w, fmt.Errorf("multiple WAV data chunks are unsupported")
			}
			w.offset, w.size = start, n
			data = true
		}
		pos = start + n + (n % 2)
	}
	if !format || !data || w.size == 0 {
		return w, failure("missing_audio", "WAV has no nonempty audio stream")
	}
	if w.size%int64(w.channels*2) != 0 {
		return w, fmt.Errorf("incomplete PCM frame")
	}
	return w, nil
}

func wavHeader(out io.Writer, frames int64) error {
	if frames <= 0 || frames > (int64(1<<32)-44)/2 {
		return fmt.Errorf("normalized audio is empty or exceeds RIFF size limit")
	}
	b := make([]byte, 44)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+frames*2))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(frames*2))
	_, err := out.Write(b)
	return err
}

func normalize(ctx context.Context, src *os.File, w wavInfo, out io.Writer) error {
	frames := w.size / int64(w.channels*2)
	count := frames * 16000 / int64(w.rate)
	if err := wavHeader(out, count); err != nil {
		return err
	}
	r := bufio.NewReader(contextReader{ctx, io.NewSectionReader(src, w.offset, w.size)})
	read := func() (int64, error) {
		var sum int64
		var b [2]byte
		for j := 0; j < w.channels; j++ {
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return 0, err
			}
			sum += int64(int16(binary.LittleEndian.Uint16(b[:])))
		}
		return sum / int64(w.channels), nil
	}
	left, err := read()
	if err != nil {
		return err
	}
	right := left
	if frames > 1 {
		right, err = read()
		if err != nil {
			return err
		}
	}
	var index int64
	bw := bufio.NewWriter(out)
	for i := int64(0); i < count; i++ {
		if i%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		pos := i * int64(w.rate)
		want := pos / 16000
		frac := pos % 16000
		for index < want {
			left = right
			index++
			if index+1 < frames {
				right, err = read()
				if err != nil {
					return err
				}
			}
		}
		sample := (left*(16000-frac) + right*frac) / 16000
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], uint16(int16(sample)))
		if _, err = bw.Write(b[:]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func decodeWAV(ctx context.Context, in c.TranscribeStageInput) (r c.TranscribeStageResult, err error) {
	r = result(in)
	p, err := inputFile(in)
	if err != nil {
		return r, err
	}
	src, err := os.Open(p)
	if err != nil {
		return r, err
	}
	defer src.Close()
	w, err := readWAV(src)
	if err != nil {
		return r, err
	}
	dir, err := workspace(in)
	if err != nil {
		return r, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	path := filepath.Join(dir, "audio.wav")
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return r, err
	}
	err = normalize(ctx, src, w, out)
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return r, err
	}
	return decoded(ctx, in, path)
}

func decoded(ctx context.Context, in c.TranscribeStageInput, path string) (c.TranscribeStageResult, error) {
	r := result(in)
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	w, err := readWAV(f)
	if err != nil {
		return r, err
	}
	if w.rate != 16000 || w.channels != 1 {
		return r, fmt.Errorf("decoder did not produce 16 kHz mono PCM16 WAV")
	}
	h, err := digest(ctx, path)
	if err != nil {
		return r, err
	}
	if r.Media == nil {
		r.Media = &c.TranscribeMedia{}
	}
	r.Media.SampleRateHz, r.Media.Channels, r.Media.DurationMS = 16000, 1, w.size*1000/32000
	r.Artifacts = []c.TranscribeArtifact{{Path: path, MediaType: "audio/wav", SHA256: h}}
	return r, nil
}

func decodeFFmpeg(ctx context.Context, in c.TranscribeStageInput, cfg Config) (r c.TranscribeStageResult, err error) {
	r = result(in)
	p, err := inputFile(in)
	if err != nil {
		return r, err
	}
	bin, err := verifiedExecutable(ctx, cfg.FFmpegPath, cfg.FFmpegSHA256)
	if err != nil {
		return r, err
	}
	dir, err := workspace(in)
	if err != nil {
		return r, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	path := filepath.Join(dir, "audio.wav")
	// No optional map: a video without audio must fail. Protocol restriction
	// keeps local playlists/media from fetching remote URLs during decoding.
	err = run(ctx, bin, "-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-n", "-protocol_whitelist", "file", "-i", p, "-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-f", "wav", path)
	if err != nil {
		return r, err
	}
	return decoded(ctx, in, path)
}
