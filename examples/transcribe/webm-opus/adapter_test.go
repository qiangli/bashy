package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func el(id uint64, b ...[]byte) []byte {
	var payload []byte
	for _, v := range b {
		payload = append(payload, v...)
	}
	var name [8]byte
	binary.BigEndian.PutUint64(name[:], id)
	i := 0
	for name[i] == 0 {
		i++
	}
	out := append([]byte{}, name[i:]...)
	n := len(payload)
	w := 1
	for uint64(n) >= (uint64(1)<<(w*7))-1 {
		w++
	}
	size := make([]byte, w)
	v := uint64(n) | (uint64(1) << (w * 7))
	for j := w - 1; j >= 0; j-- {
		size[j] = byte(v)
		v >>= 8
	}
	out = append(out, size...)
	return append(out, payload...)
}
func u(id, v uint64) []byte           { var b [8]byte; binary.BigEndian.PutUint64(b[:], v); return el(id, b[:]) }
func f64(id uint64, v float64) []byte { return u(id, math.Float64bits(v)) }
func head(ch int, skip uint16) []byte {
	b := make([]byte, 19)
	copy(b, "OpusHead")
	b[8] = 1
	b[9] = byte(ch)
	binary.LittleEndian.PutUint16(b[10:], skip)
	binary.LittleEndian.PutUint32(b[12:], 48000)
	return b
}
func trackEl(num uint64, ch int, skip uint16, extra ...[]byte) []byte {
	parts := [][]byte{u(0xd7, num), u(0x73c5, num), u(0x83, 2), el(0x86, []byte("A_OPUS")), el(0x63a2, head(ch, skip)), u(0x56aa, uint64(skip)*1000000000/48000), el(0xe1, f64(0xb5, 48000), u(0x9f, uint64(ch)))}
	return el(0xae, append(parts, extra...)...)
}
func videoTrack() []byte {
	return el(0xae, u(0xd7, 2), u(0x73c5, 2), u(0x83, 1), el(0x86, []byte("V_VP9")))
}
func rawBlock(track byte, ts int16, flags byte, packets ...[]byte) []byte {
	b := []byte{0x80 | track, byte(uint16(ts) >> 8), byte(ts), flags}
	for _, p := range packets {
		b = append(b, p...)
	}
	return b
}
func simple(ts int16, p []byte) []byte { return el(0xa3, rawBlock(1, ts, 0x80, p)) }
func fixture(tracks []byte, blocks ...[]byte) []byte {
	return elDoc(tracks, 1000000, el(0x1f43b675, append([][]byte{u(0xe7, 0)}, blocks...)...))
}
func elDoc(tracks []byte, scale uint64, clusters ...[]byte) []byte {
	hdr := el(0x1a45dfa3, el(0x4282, []byte("webm")), u(0x42f7, 1), u(0x4285, 2))
	parts := [][]byte{el(0x1549a966, u(0x2ad7b1, scale)), el(0x1654ae6b, tracks)}
	return append(hdr, el(0x18538067, append(parts, clusters...)...)...)
}

var silence = []byte{0xf8, 0xff, 0xfe}

func requestFor(t *testing.T, data []byte) request {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "speech ; 日本.webm")
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	work := filepath.Join(dir, "work")
	if e := os.Mkdir(work, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(work, "unrelated"), []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	return request{Version: version, Stage: "decode", WorkDir: work, Artifacts: []artifact{{Path: path}}, Media: map[string]any{"source": "fixture", "video_id": "fixture-id"}}
}
func assertPreserved(t *testing.T, in request, before []byte, success bool) {
	t.Helper()
	data, e := os.ReadFile(in.Artifacts[0].Path)
	if e != nil || !bytes.Equal(data, before) {
		t.Fatal("input changed", e)
	}
	data, e = os.ReadFile(filepath.Join(in.WorkDir, "unrelated"))
	if e != nil || string(data) != "keep" {
		t.Fatal("unrelated file changed", e)
	}
	entries, e := os.ReadDir(in.WorkDir)
	want := 1
	if success {
		want = 2
	}
	if e != nil || len(entries) != want {
		t.Fatalf("owned cleanup: %v %v", entries, e)
	}
}
func output(t *testing.T, in request, frames int) []byte {
	t.Helper()
	res, s := execute(context.Background(), in)
	if s != 0 || res.Error != nil {
		t.Fatalf("status %d result %+v", s, res.Error)
	}
	data, e := os.ReadFile(res.Artifacts[0].Path)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(data[:44], wavHeader(int64(frames))) || len(data) != 44+frames*2 {
		t.Fatalf("WAV frames got %d want %d", (len(data)-44)/2, frames)
	}
	sum := sha256.Sum256(data)
	if res.Artifacts[0].SHA256 != hex.EncodeToString(sum[:]) || res.Media["video_id"] != "fixture-id" || res.Media["sample_rate_hz"] != 16000 {
		t.Fatal("stage metadata/hash")
	}
	if _, e = os.Stat(filepath.Join(filepath.Dir(res.Artifacts[0].Path), "audio.partial")); !os.IsNotExist(e) {
		t.Fatal("partial published")
	}
	return data[44:]
}
func TestTrimVideoAndAllLacing(t *testing.T) {
	for _, mode := range []byte{0, 2, 4, 6} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			var b []byte
			frames := 320
			switch mode {
			case 0:
				b = rawBlock(1, 0, 0, silence)
			case 2:
				b = rawBlock(1, 0, mode, []byte{1, 3}, silence, silence)
				frames = 640
			case 4:
				b = rawBlock(1, 0, mode, []byte{1}, silence, silence)
				frames = 640
			case 6:
				b = rawBlock(1, 0, mode, []byte{1, 0x83}, silence, silence)
				frames = 640
			}
			block := el(0xa0, el(0xa1, b), u(0x75a2, 10000000))
			data := fixture(append(trackEl(1, 1, 312), videoTrack()...), el(0xa3, rawBlock(2, 0, 0x88, []byte{1, 2, 3})), block)
			in := requestFor(t, data)
			output(t, in, frames-104-160)
			assertPreserved(t, in, data, true)
		})
	}
}
func TestNegativePaddingAndNonzeroOrigin(t *testing.T) {
	padding := int64(-10000000)
	data := fixture(trackEl(1, 1, 0), el(0xa0, el(0xa1, rawBlock(1, 700, 0, silence)), u(0x75a2, uint64(padding))))
	output(t, requestFor(t, data), 160)
}
func TestMalformedRejectsAndPreserves(t *testing.T) {
	valid := fixture(trackEl(1, 1, 0), simple(0, silence))
	badHead := head(1, 0)
	badHead[18] = 1
	cases := map[string][]byte{
		"truncated": valid[:len(valid)-1], "unknown-track": fixture(trackEl(1, 1, 0), el(0xa3, rawBlock(3, 0, 0x80, silence))),
		"ambiguous":      fixture(append(trackEl(1, 1, 0), trackEl(2, 1, 0)...), simple(0, silence)),
		"encryption":     fixture(trackEl(1, 1, 0, el(0x6d80, []byte{1})), simple(0, silence)),
		"duplicate-head": fixture(trackEl(1, 1, 0, el(0x63a2, badHead)), simple(0, silence)),
		"bad-lace":       fixture(trackEl(1, 1, 0), el(0xa3, rawBlock(1, 0, 0x82, []byte{2, 255}))),
		"gap":            fixture(trackEl(1, 1, 0), simple(0, silence), simple(200, silence)),
		"overlap":        fixture(trackEl(1, 1, 0), simple(0, silence), simple(0, silence)),
		"bad-codec":      fixture(trackEl(1, 1, 0), simple(0, []byte{3, 0})),
		"padding":        fixture(trackEl(1, 1, 0), el(0xa0, el(0xa1, rawBlock(1, 0, 0, silence)), u(0x75a2, 30000000))),
		"oversize-leaf":  fixture(trackEl(1, 1, 0), el(0xec, make([]byte, maxLeaf+1))),
	}
	deep := simple(0, silence)
	for i := 0; i < maxDepth+2; i++ {
		deep = el(0xa0, deep)
	}
	cases["depth"] = fixture(trackEl(1, 1, 0), deep)
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			in := requestFor(t, data)
			res, s := execute(context.Background(), in)
			if s == 0 || res.Error == nil || len(res.Artifacts) != 0 {
				t.Fatalf("accepted invalid: %+v", res)
			}
			assertPreserved(t, in, data, false)
		})
	}
}

// Deterministic cancellation at context observation points exercises the actual
// synchronous parser and decoder without sleep races or worker goroutines.
type cancelAfter struct {
	context.Context
	left int
}

func (c *cancelAfter) Err() error {
	c.left--
	if c.left <= 0 {
		return context.Canceled
	}
	return nil
}
func TestCancellationAtEveryPhase(t *testing.T) {
	blocks := make([][]byte, 30)
	for i := range blocks {
		blocks[i] = simple(int16(i*20), silence)
	}
	data := fixture(trackEl(1, 1, 0), blocks...)
	cancelled := 0
	succeeded := 0
	for _, budget := range []int{1, 2, 3, 10, 30, 60, 80, 90, 100, 120, 150, 200, 300, 500} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			in := requestFor(t, data)
			ctx := &cancelAfter{Context: context.Background(), left: budget}
			res, s := execute(ctx, in)
			if s == 130 {
				cancelled++
				if res.Error.Code != "cancelled" || len(res.Artifacts) != 0 {
					t.Fatal(res)
				}
				assertPreserved(t, in, data, false)
			} else if s == 0 {
				succeeded++
				assertPreserved(t, in, data, true)
			} else {
				t.Fatalf("unexpected %d %+v", s, res)
			}
		})
	}
	if cancelled < 5 || succeeded == 0 {
		t.Fatalf("cancellation coverage %d/%d", cancelled, succeeded)
	}
}
func TestStageJSONAndHash(t *testing.T) {
	data := fixture(trackEl(1, 1, 0), simple(0, silence))
	in := requestFor(t, data)
	in.Artifacts[0].SHA256 = "bad"
	res, s := execute(context.Background(), in)
	if s == 0 || res.Error == nil {
		t.Fatal("hash mismatch accepted")
	}
	assertPreserved(t, in, data, false)
	in.Artifacts[0].SHA256 = ""
	b, _ := json.Marshal(in)
	req := filepath.Join(t.TempDir(), "request.json")
	os.WriteFile(req, b, 0600)
	var out bytes.Buffer
	if run(context.Background(), []string{req}, &out) != 0 {
		t.Fatal(out.String())
	}
	if json.Unmarshal(out.Bytes(), &res) != nil || len(res.Artifacts) != 1 || res.Version != version {
		t.Fatal(out.String())
	}
}
func TestUnknownSizeClusters(t *testing.T) {
	cl := func(ts uint64) []byte {
		b := []byte{0x1f, 0x43, 0xb6, 0x75, 0xff}
		b = append(b, u(0xe7, ts)...)
		return append(b, simple(0, silence)...)
	}
	data := elDoc(trackEl(1, 1, 0), 1000000, cl(0), cl(20))
	output(t, requestFor(t, data), 640)
}
func TestLimits(t *testing.T) {
	if _, err := boundedRead(context.Background(), bytes.NewReader(make([]byte, 11)), 10); err == nil {
		t.Fatal("input limit")
	}
	p := parser{ctx: context.Background(), count: maxElements}
	if _, _, e := p.list(el(0xec, []byte{1}), 0, false); e == nil {
		t.Fatal("element limit")
	}
	if _, e := decodePacket(nil, make([]byte, 65537), nil); e == nil {
		t.Fatal("packet limit")
	}
	if _, e := decodePacket(nil, silence, nil); e == nil {
		t.Fatal("panic boundary")
	}
}
func FuzzDemux(f *testing.F) {
	f.Add(fixture(trackEl(1, 1, 0), simple(0, silence)))
	f.Add([]byte{0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		m, e := demux(context.Background(), data)
		if e == nil {
			_, _ = decode(context.Background(), m, io.Discard)
		}
	})
}
func TestCancelledReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := boundedRead(ctx, bytes.NewReader(nil), 10)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestSpeechAndStereoAgainstIndependentPCM(t *testing.T) {
	for _, name := range []string{"speech", "stereo"} {
		t.Run(name, func(t *testing.T) {
			data, e := os.ReadFile("testdata/" + name + ".webm")
			if e != nil {
				t.Fatal(e)
			}
			ref, e := os.ReadFile("testdata/" + name + "-reference.pcm")
			if e != nil {
				t.Fatal(e)
			}
			in := requestFor(t, data)
			got := output(t, in, len(ref)/2)
			assertPreserved(t, in, data, true)
			var power, noise float64
			nonzero := 0
			for i := 0; i < len(ref); i += 2 {
				a := float64(int16(binary.LittleEndian.Uint16(ref[i:])))
				b := float64(int16(binary.LittleEndian.Uint16(got[i:])))
				power += a * a
				noise += (a - b) * (a - b)
				if b != 0 {
					nonzero++
				}
			}
			snr := 10 * math.Log10(power/math.Max(noise, 1))
			rms := math.Sqrt(power / float64(len(ref)/2))
			t.Logf("%d samples, %d nonzero, reference RMS %.2f, SNR %.2f dB", len(ref)/2, nonzero, rms, snr)
			if nonzero < len(ref)/4 || rms < 100 || snr < 20 {
				t.Fatalf("non-silent reference fidelity failed: %.2f dB", snr)
			}
		})
	}
}

func TestPacketAndOutputBudgets(t *testing.T) {
	data := fixture(trackEl(1, 1, 0), simple(0, silence))
	m, e := demux(context.Background(), data)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if _, e = decodeLimited(context.Background(), m, &out, 319); e == nil || out.Len() != 0 {
		t.Fatalf("output limit: %d %v", out.Len(), e)
	}
	lace := []byte{255}
	for i := 0; i < 256; i++ {
		lace = append(lace, silence...)
	}
	block := el(0xa3, rawBlock(1, 0, 0x84, lace))
	blocks := make([][]byte, maxPackets/256+1)
	for i := range blocks {
		blocks[i] = block
	}
	_, e = demux(context.Background(), fixture(trackEl(1, 1, 0), blocks...))
	if e == nil || e.Error() != "packet count limit" {
		t.Fatalf("packet count: %v", e)
	}
}
func TestStructuralAndMappingRejections(t *testing.T) {
	base := fixture(trackEl(1, 1, 0), simple(0, silence))
	mapping := append([]byte{}, base...)
	i := bytes.Index(mapping, []byte("OpusHead"))
	mapping[i+18] = 1
	mismatch := append([]byte{}, base...)
	i = bytes.Index(mismatch, []byte("OpusHead"))
	mapping2 := append([]byte{}, mismatch...)
	mapping2[i+9] = 2
	delay := append([]byte{}, base...)
	i = bytes.Index(delay, []byte("OpusHead"))
	delay[i+10] = 10
	cases := map[string][]byte{
		"mapping": mapping, "channels": mapping2, "delay": delay,
		"misplaced-block":    fixture(trackEl(1, 1, 0), simple(0, silence), el(0xa0, simple(20, silence))),
		"encrypted-block":    fixture(trackEl(1, 1, 0), simple(0, silence), el(0xaf, []byte{1})),
		"lace-disabled":      fixture(trackEl(1, 1, 0, u(0x9c, 0)), el(0xa3, rawBlock(1, 0, 0x84, []byte{1}, silence, silence))),
		"timestamp-overflow": elDoc(trackEl(1, 1, 0), 1, el(0x1f43b675, u(0xe7, math.MaxInt64), simple(1, silence))),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, e := demux(context.Background(), data); e == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
}
func TestEBMLVariableAndXiphLaces(t *testing.T) {
	// Three distinct lengths; the second EBML size is a negative signed delta.
	a, b, c := bytes.Repeat([]byte{1}, 260), []byte{2, 3}, []byte{4, 5, 6}
	for _, tc := range []struct {
		flags byte
		sizes []byte
	}{{0x82, []byte{2, 255, 5, 2}}, {0x86, []byte{2, 0x41, 0x04, 0x5e, 0xfd}}} {
		block, e := parseBlock(rawBlock(1, -2, tc.flags, tc.sizes, a, b, c), true)
		if e != nil {
			t.Fatal(e)
		}
		if block.time != -2 || len(block.packets) != 3 || !bytes.Equal(block.packets[0], a) || !bytes.Equal(block.packets[1], b) || !bytes.Equal(block.packets[2], c) {
			t.Fatal("incorrect delacing")
		}
	}
}
func TestOpusGain(t *testing.T) {
	data, e := os.ReadFile("testdata/speech.webm")
	if e != nil {
		t.Fatal(e)
	}
	plain := output(t, requestFor(t, data), 31896)
	i := bytes.Index(data, []byte("OpusHead"))
	binary.LittleEndian.PutUint16(data[i+16:], uint16(6*256))
	gained := output(t, requestFor(t, data), 31896)
	ratio := math.Pow(10, 6.0/20)
	for i := 0; i < len(plain); i += 2 {
		a := float64(int16(binary.LittleEndian.Uint16(plain[i:])))
		b := float64(int16(binary.LittleEndian.Uint16(gained[i:])))
		want := math.Max(-32768, math.Min(32767, a*ratio))
		if math.Abs(want-b) > 2 {
			t.Fatalf("gain sample %d: %f want %f", i/2, b, want)
		}
	}
}
func TestUnknownSegmentAndScaledTime(t *testing.T) {
	data := elDoc(trackEl(1, 1, 0), 500000, el(0x1f43b675, u(0xe7, 100), simple(0, silence), simple(40, silence)))
	i := bytes.Index(data, []byte{0x18, 0x53, 0x80, 0x67})
	_, n, e := vint(data[i+4:], false)
	if e != nil {
		t.Fatal(e)
	}
	unknown := append(append(append([]byte{}, data[:i+4]...), 0xff), data[i+4+n:]...)
	output(t, requestFor(t, unknown), 640)
}
func TestInvalidRequestAndOutputFailure(t *testing.T) {
	data := fixture(trackEl(1, 1, 0), simple(0, silence))
	in := requestFor(t, data)
	in.Version = "wrong"
	res, s := execute(context.Background(), in)
	if s != 2 || res.Error.Code != "invalid_request" {
		t.Fatal(res, s)
	}
	assertPreserved(t, in, data, false)
	in.Version = version
	b, _ := json.Marshal(in)
	req := filepath.Join(t.TempDir(), "request.json")
	os.WriteFile(req, b, 0600)
	if run(context.Background(), []string{req}, failingWriter{}) != 1 {
		t.Fatal("output failure not reported")
	}
	assertPreserved(t, in, data, false)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func FuzzOpusPacket(f *testing.F) {
	f.Add(silence)
	f.Add([]byte{3, 0})
	f.Fuzz(func(t *testing.T, p []byte) {
		if len(p) > 65536 {
			return
		}
		m := media{track: track{gain: 1}, scale: 1000000, blocks: []block{{packets: [][]byte{p}}}}
		_, _ = decode(context.Background(), m, io.Discard)
	})
}

func TestFixtureDigests(t *testing.T) {
	data, e := os.ReadFile("testdata/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		Files map[string]string `json:"files"`
	}
	if e = json.Unmarshal(data, &m); e != nil || len(m.Files) != 4 {
		t.Fatal("fixture manifest", e)
	}
	for name, want := range m.Files {
		data, e = os.ReadFile(filepath.Join("testdata", name))
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatal("fixture hash", name)
		}
	}
}
func TestLateInvalidPacketDoesNotPublish(t *testing.T) {
	data := fixture(trackEl(1, 1, 0), simple(0, silence), simple(20, []byte{3, 0}))
	in := requestFor(t, data)
	res, s := execute(context.Background(), in)
	if s == 0 || res.Error == nil || len(res.Artifacts) != 0 {
		t.Fatal("late failure published")
	}
	assertPreserved(t, in, data, false)
}
func TestRunPreCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if s := run(ctx, []string{"unused.json"}, &out); s != 130 {
		t.Fatal(s, out.String())
	}
	var res result
	if e := json.Unmarshal(out.Bytes(), &res); e != nil || res.Error.Code != "cancelled" {
		t.Fatal(out.String())
	}
}
