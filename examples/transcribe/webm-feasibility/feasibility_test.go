// These are dependency characterization tests, not adapter acceptance tests.
package feasibility

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/at-wat/ebml-go/webm"
	"github.com/pion/opus"
)

type blockGroup struct {
	Block          ebml.Block
	DiscardPadding int64 `ebml:"DiscardPadding,size=8"`
}
type cluster struct {
	Timecode   uint64
	BlockGroup []blockGroup
}
type document struct {
	EBML    webm.EBMLHeader
	Segment struct {
		Info    webm.Info
		Tracks  webm.Tracks
		Cluster []cluster
	} `ebml:"Segment,size=unknown"`
}

// Authored, synthetic 20 ms CELT silence packet; no downloaded media, codec
// executable, encoder, model or upstream test-data licensing is involved.
var silence = []byte{0xf8, 0xff, 0xfe}

func fixture(t *testing.T, blockTrack uint64, padding int64) []byte {
	t.Helper()
	var d document
	d.EBML = webm.EBMLHeader{EBMLVersion: 1, EBMLReadVersion: 1, EBMLMaxIDLength: 4, EBMLMaxSizeLength: 8, DocType: "webm", DocTypeVersion: 4, DocTypeReadVersion: 2}
	d.Segment.Info.TimecodeScale = 1000000
	d.Segment.Tracks.TrackEntry = []webm.TrackEntry{{TrackNumber: 1, TrackUID: 1, TrackType: 2, CodecID: "A_OPUS", CodecPrivate: []byte{'O', 'p', 'u', 's', 'H', 'e', 'a', 'd', 1, 1, 0, 0, 0x80, 0xbb, 0, 0, 0, 0, 0}, Audio: &webm.Audio{SamplingFrequency: 48000, Channels: 1}}}
	d.Segment.Cluster = []cluster{{BlockGroup: []blockGroup{{Block: ebml.Block{TrackNumber: blockTrack, Keyframe: true, Data: [][]byte{silence}}, DiscardPadding: padding}}}}
	var b bytes.Buffer
	if err := ebml.Marshal(&d, &b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func readers(t *testing.T, r io.Reader) []mkvcore.BlockReadCloserWithTrackEntry {
	t.Helper()
	rs, err := mkvcore.NewSimpleBlockReader(r,
		mkvcore.WithUnmarshalOptions(ebml.WithMaxLeafElementSize(1<<20)),
		mkvcore.WithOnFatalHandler(func(err error) { t.Errorf("parser: %v", err) }))
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestPion16kMonoControlledPacket(t *testing.T) {
	d, err := opus.NewDecoderWithOutput(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int16, 1920) // maximum 120 ms packet at 16 kHz
	n, err := d.DecodeToInt16(silence, out)
	if err != nil || n != 320 {
		t.Fatalf("samples=%d err=%v", n, err)
	}
	for _, s := range out[:n] {
		if s != 0 {
			t.Fatalf("non-silence sample: %d", s)
		}
	}
	if _, err := d.DecodeToInt16(nil, out); err == nil {
		t.Fatal("empty packet accepted")
	}
	if _, err := d.DecodeToInt16(silence, out[:1]); err == nil {
		t.Fatal("undersized output accepted")
	}
}

func TestSimpleReaderLosesDiscardPadding(t *testing.T) {
	var packets [][]byte
	for _, padding := range []int64{0, 10000000} {
		data := fixture(t, 1, padding)
		var full document
		if err := ebml.Unmarshal(bytes.NewReader(data), &full); err != nil {
			t.Fatal(err)
		}
		if got := full.Segment.Cluster[0].BlockGroup[0].DiscardPadding; got != padding {
			t.Fatalf("fixture padding=%d", got)
		}
		rs := readers(t, bytes.NewReader(data))
		packet, _, ts, err := rs[0].Read()
		if err != nil || ts != 0 {
			t.Fatalf("timestamp=%d err=%v", ts, err)
		}
		packets = append(packets, packet)
		if _, _, _, err := rs[0].Read(); err != io.EOF {
			t.Fatalf("end=%v", err)
		}
		if err := rs[0].Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(packets[0], packets[1]) {
		t.Fatal("expected identical packets despite different trim")
	}
	t.Log("0 ms and 10 ms discard padding produce identical convenience-reader output")
}

func TestUnknownTrackPanicsOutsideCaller(t *testing.T) {
	if os.Getenv("WEBM_FEASIBILITY_CRASH_CHILD") == "1" {
		rs := readers(t, bytes.NewReader(fixture(t, 2, 0)))
		// The callback and caller's recover cannot catch the dispatcher goroutine.
		rs[0].Read()
		t.Fatal("expected upstream dispatcher panic")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUnknownTrackPanicsOutsideCaller$")
	cmd.Env = append(os.Environ(), "WEBM_FEASIBILITY_CRASH_CHILD=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil || err == nil || !bytes.Contains(out, []byte("panic: runtime error: invalid memory address")) || !bytes.Contains(out, []byte("mkvcore.NewSimpleBlockReader.func")) {
		t.Fatalf("expected dispatcher nil-track panic, err=%v output=%s", err, out)
	}
	t.Log("undeclared track crashes dispatcher despite custom onFatal callback")
}

type gatedReader struct {
	r                        io.Reader
	entered, release, exited chan struct{}
}

func (g *gatedReader) Read(p []byte) (int, error) {
	n, err := g.r.Read(p)
	if err == io.EOF {
		close(g.entered)
		<-g.release
		close(g.exited)
	}
	return n, err
}

func TestTrackCloseDoesNotCancelSource(t *testing.T) {
	g := &gatedReader{r: bytes.NewReader(fixture(t, 1, 0)), entered: make(chan struct{}), release: make(chan struct{}), exited: make(chan struct{})}
	rs := readers(t, g)
	defer close(g.release) // always let the upstream parser terminate
	if _, _, _, err := rs[0].Read(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-g.entered:
	case <-time.After(time.Second):
		t.Fatal("parser did not reach source gate")
	}
	if err := rs[0].Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-g.exited:
		t.Fatal("source unexpectedly stopped")
	default:
	}
	t.Log("track Close returns while parser remains inside source Read; caller must cancel/close source separately")
}

func TestLeafAllocationCanBeBounded(t *testing.T) {
	// CodecPrivate (0x63a2), a 16-byte payload. No large allocation is attempted.
	input := append([]byte{0x63, 0xa2, 0x90}, bytes.Repeat([]byte{0}, 16)...)
	var target struct{ CodecPrivate []byte }
	err := ebml.Unmarshal(bytes.NewReader(input), &target, ebml.WithMaxLeafElementSize(8))
	if !errors.Is(err, ebml.ErrTooLargeElement) {
		t.Fatalf("limit error=%v", err)
	}
}

func TestTruncatedContainerReportedSeparatelyFromEOF(t *testing.T) {
	data := fixture(t, 1, 0)
	fatal := make(chan error, 1)
	rs, err := mkvcore.NewSimpleBlockReader(bytes.NewReader(data[:len(data)-1]), mkvcore.WithOnFatalHandler(func(err error) { fatal <- err }))
	if err != nil {
		t.Fatal(err)
	}
	defer rs[0].Close()
	for {
		_, _, _, err = rs[0].Read()
		if err != nil {
			break
		}
	}
	if err != io.EOF {
		t.Fatalf("reader error=%v", err)
	}
	select {
	case err := <-fatal:
		if err == nil || !strings.Contains(err.Error(), "EOF") {
			t.Fatalf("fatal=%v", err)
		}
		t.Logf("Read returns EOF; separate fatal callback reports %v", err)
	case <-time.After(time.Second):
		t.Fatal("missing parser error callback")
	}
}
