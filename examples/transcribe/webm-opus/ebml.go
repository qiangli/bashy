package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

// All payload slices alias the bounded input. No parser goroutines, callbacks,
// reflection, or allocations based on unvalidated EBML sizes.
const maxInput = 128 << 20
const maxLeaf = 8 << 20
const maxElements = 100000
const maxPackets = 300000
const maxOutput = 256 << 20
const maxDepth = 16

type element struct {
	id       uint64
	data     []byte
	children []element
}
type parser struct {
	ctx   context.Context
	count int
}

var masters = map[uint64]bool{0x1a45dfa3: true, 0x18538067: true, 0x1549a966: true, 0x1654ae6b: true, 0xae: true, 0xe1: true, 0xe0: true, 0x1f43b675: true, 0xa0: true}
var levelOne = map[uint64]bool{0x114d9b74: true, 0x1549a966: true, 0x1654ae6b: true, 0x1f43b675: true, 0x1c53bb6b: true, 0x1941a469: true, 0x1043a770: true, 0x1254c367: true}

// Known structural/timing fields must occur at their defined level. Unknown
// finite metadata is skipped as an opaque bounded leaf, never interpreted.
var parentOf = map[uint64]uint64{
	0x1a45dfa3: 0, 0x18538067: 0,
	0x1549a966: 0x18538067, 0x1654ae6b: 0x18538067, 0x1f43b675: 0x18538067,
	0xae: 0x1654ae6b, 0xe1: 0xae, 0xe0: 0xae, 0xa0: 0x1f43b675,
	0xa3: 0x1f43b675, 0xa1: 0xa0, 0x75a2: 0xa0, 0xe7: 0x1f43b675,
	0x2ad7b1: 0x1549a966, 0xd7: 0xae, 0x73c5: 0xae, 0x83: 0xae, 0x86: 0xae,
	0x63a2: 0xae, 0x56aa: 0xae, 0x9f: 0xe1, 0xb5: 0xe1,
}

func validateStructure(ctx context.Context, es []element, parent uint64) error {
	for _, e := range es {
		if err := ctx.Err(); err != nil {
			return err
		}
		if want, known := parentOf[e.id]; known && want != parent {
			return fmt.Errorf("misplaced EBML element %x", e.id)
		}
		if e.id == 0xaf {
			return fmt.Errorf("EncryptedBlock unsupported")
		}
		if err := validateStructure(ctx, e.children, e.id); err != nil {
			return err
		}
	}
	return nil
}

func vint(b []byte, id bool) (uint64, int, error) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, fmt.Errorf("invalid/truncated EBML VINT")
	}
	n := 1
	mask := byte(0x80)
	for b[0]&mask == 0 {
		n++
		mask >>= 1
	}
	if n > len(b) || (id && n > 4) {
		return 0, 0, fmt.Errorf("truncated/oversized EBML VINT")
	}
	v := uint64(b[0])
	if !id {
		v = uint64(b[0] &^ mask)
	}
	for _, x := range b[1:n] {
		v = v<<8 | uint64(x)
	}
	return v, n, nil
}
func (p *parser) list(b []byte, depth int, unknownCluster bool) ([]element, int, error) {
	if depth > maxDepth {
		return nil, 0, fmt.Errorf("EBML depth limit")
	}
	var out []element
	pos := 0
	for pos < len(b) {
		if err := p.ctx.Err(); err != nil {
			return nil, 0, err
		}
		id, ni, err := vint(b[pos:], true)
		if err != nil {
			return nil, 0, err
		}
		if unknownCluster && levelOne[id] {
			break
		}
		size, ns, err := vint(b[pos+ni:], false)
		if err != nil {
			return nil, 0, err
		}
		p.count++
		if p.count > maxElements {
			return nil, 0, fmt.Errorf("EBML element limit")
		}
		start := pos + ni + ns
		unknown := size == (uint64(1)<<(7*ns))-1
		if unknown && id != 0x18538067 && id != 0x1f43b675 {
			return nil, 0, fmt.Errorf("unknown size only supported for Segment/Cluster")
		}
		if !unknown && size > uint64(len(b)-start) {
			return nil, 0, fmt.Errorf("truncated EBML payload")
		}
		end := len(b)
		if !unknown {
			end = start + int(size)
		}
		e := element{id: id, data: b[start:end]}
		if masters[id] {
			var used int
			e.children, used, err = p.list(e.data, depth+1, unknown && id == 0x1f43b675)
			if err != nil {
				return nil, 0, err
			}
			end = start + used
			e.data = b[start:end]
		} else if len(e.data) > maxLeaf {
			return nil, 0, fmt.Errorf("EBML leaf limit")
		}
		out = append(out, e)
		pos = end
	}
	return out, pos, nil
}
func one(es []element, id uint64, required bool) (element, error) {
	var e element
	found := false
	for _, x := range es {
		if x.id == id {
			if found {
				return e, fmt.Errorf("duplicate EBML field %x", id)
			}
			e = x
			found = true
		}
	}
	if required && !found {
		return e, fmt.Errorf("missing EBML field %x", id)
	}
	return e, nil
}
func uintValue(e element) (uint64, error) {
	if len(e.data) < 1 || len(e.data) > 8 {
		return 0, fmt.Errorf("invalid integer %x", e.id)
	}
	var v uint64
	for _, b := range e.data {
		v = v<<8 | uint64(b)
	}
	return v, nil
}
func number(es []element, id uint64, def uint64, required bool) (uint64, error) {
	e, err := one(es, id, required)
	if err != nil {
		return 0, err
	}
	if e.id == 0 {
		return def, nil
	}
	return uintValue(e)
}
func signed(e element) (int64, error) {
	v, err := uintValue(e)
	if err != nil {
		return 0, err
	}
	n := len(e.data) * 8
	if n < 64 && v&(uint64(1)<<(n-1)) != 0 {
		v |= math.MaxUint64 << n
	}
	return int64(v), nil
}

type track struct {
	number, uid, kind, delay uint64
	lacing                   bool
	head                     []byte
	channels                 int
	skip                     int
	gain                     float64
}
type block struct {
	track     uint64
	time      int64
	padding   int64
	packets   [][]byte
	invisible bool
}
type media struct {
	track  track
	scale  uint64
	blocks []block
}

func demux(ctx context.Context, data []byte) (media, error) {
	var m media
	if len(data) > maxInput {
		return m, fmt.Errorf("input limit")
	}
	p := parser{ctx: ctx}
	roots, _, err := p.list(data, 0, false)
	if err != nil {
		return m, err
	}
	if len(roots) != 2 || roots[0].id != 0x1a45dfa3 || roots[1].id != 0x18538067 {
		return m, fmt.Errorf("expected one EBML header and Segment")
	}
	if err = validateStructure(ctx, roots, 0); err != nil {
		return m, err
	}
	h := roots[0].children
	doc, err := one(h, 0x4282, true)
	if err != nil || string(doc.data) != "webm" {
		return m, fmt.Errorf("expected WebM DocType")
	}
	for _, r := range []struct{ id, max, def uint64 }{{0x42f7, 1, 1}, {0x42f2, 4, 4}, {0x42f3, 8, 8}, {0x4285, 4, 1}} {
		v, e := number(h, r.id, r.def, false)
		if e != nil || v < 1 || v > r.max {
			return m, fmt.Errorf("unsupported EBML header")
		}
	}
	seg := roots[1].children
	info, err := one(seg, 0x1549a966, true)
	if err != nil {
		return m, err
	}
	m.scale, err = number(info.children, 0x2ad7b1, 1000000, false)
	if err != nil || m.scale == 0 || m.scale > 1000000000 {
		return m, fmt.Errorf("invalid TimecodeScale")
	}
	ts, err := one(seg, 0x1654ae6b, true)
	if err != nil {
		return m, err
	}
	tracks := map[uint64]track{}
	uids := map[uint64]bool{}
	audio := 0
	for _, e := range ts.children {
		if err := ctx.Err(); err != nil {
			return m, err
		}
		if e.id != 0xae {
			continue
		}
		if len(tracks) >= 32 {
			return m, fmt.Errorf("track limit")
		}
		enabled, e2 := number(e.children, 0xb9, 1, false)
		if e2 != nil || enabled != 1 {
			return m, fmt.Errorf("disabled/invalid track unsupported")
		}
		t := track{}
		t.number, err = number(e.children, 0xd7, 0, true)
		if err != nil || t.number == 0 {
			return m, fmt.Errorf("invalid track number")
		}
		if _, ok := tracks[t.number]; ok {
			return m, fmt.Errorf("duplicate track number")
		}
		t.uid, err = number(e.children, 0x73c5, 0, true)
		if err != nil || t.uid == 0 || uids[t.uid] {
			return m, fmt.Errorf("invalid/duplicate track UID")
		}
		uids[t.uid] = true
		lace, e2 := number(e.children, 0x9c, 1, false)
		if e2 != nil || lace > 1 {
			return m, fmt.Errorf("invalid FlagLacing")
		}
		t.lacing = lace == 1
		t.kind, err = number(e.children, 0x83, 0, true)
		if err != nil {
			return m, err
		}
		for _, x := range e.children {
			if x.id == 0x6d80 || x.id == 0x6fab || x.id == 0x6624 || x.id == 0x23314f || x.id == 0x537f {
				return m, fmt.Errorf("encoded/encrypted or transformed track unsupported")
			}
		}
		if t.kind == 2 {
			audio++
			codec, e2 := one(e.children, 0x86, true)
			if e2 != nil || string(codec.data) != "A_OPUS" {
				return m, fmt.Errorf("audio track is not Opus")
			}
			head, e2 := one(e.children, 0x63a2, true)
			if e2 != nil {
				return m, e2
			}
			t.head = head.data
			if len(t.head) != 19 || string(t.head[:8]) != "OpusHead" || t.head[8] != 1 || t.head[18] != 0 || (t.head[9] != 1 && t.head[9] != 2) {
				return m, fmt.Errorf("unsupported OpusHead version/channel mapping")
			}
			t.channels = int(t.head[9])
			t.skip = int(binary.LittleEndian.Uint16(t.head[10:12]))
			t.gain = math.Pow(10, float64(int16(binary.LittleEndian.Uint16(t.head[16:18])))/5120)
			t.delay, e2 = number(e.children, 0x56aa, 0, true)
			if e2 != nil {
				return m, e2
			}
			// CodecDelay and Opus pre-skip describe the same delay, not two trims.
			expected := uint64(t.skip) * 1000000000 / 48000
			if t.delay > expected+1 || expected > t.delay+1 {
				return m, fmt.Errorf("CodecDelay disagrees with OpusHead pre-skip")
			}
			a, e2 := one(e.children, 0xe1, true)
			if e2 != nil {
				return m, e2
			}
			for _, x := range a.children {
				if x.id == 0x7d7b || x.id == 0x78b5 {
					return m, fmt.Errorf("audio output mapping unsupported")
				}
			}
			ch, e2 := number(a.children, 0x9f, 1, false)
			if e2 != nil || int(ch) != t.channels {
				return m, fmt.Errorf("channel metadata mismatch")
			}
			freq, e2 := one(a.children, 0xb5, true)
			if e2 != nil {
				return m, e2
			}
			var rate float64
			if len(freq.data) == 4 {
				rate = float64(math.Float32frombits(binary.BigEndian.Uint32(freq.data)))
			} else if len(freq.data) == 8 {
				rate = math.Float64frombits(binary.BigEndian.Uint64(freq.data))
			}
			if rate != 48000 {
				return m, fmt.Errorf("Opus sampling frequency must be 48000")
			}
			m.track = t
		} else if t.kind != 1 {
			return m, fmt.Errorf("unsupported track type")
		}
		tracks[t.number] = t
	}
	if audio != 1 {
		return m, fmt.Errorf("expected exactly one Opus audio track")
	}
	packets := 0
	for _, cl := range seg {
		if err := ctx.Err(); err != nil {
			return m, err
		}
		if cl.id != 0x1f43b675 {
			continue
		}
		clock, e := number(cl.children, 0xe7, 0, true)
		if e != nil || clock > math.MaxInt64/m.scale {
			return m, fmt.Errorf("invalid cluster timestamp")
		}
		for _, child := range cl.children {
			if err := ctx.Err(); err != nil {
				return m, err
			}
			be := child
			var padding int64
			if child.id == 0xa0 {
				be, err = one(child.children, 0xa1, true)
				if err != nil {
					return m, err
				}
				dp, e := one(child.children, 0x75a2, false)
				if e != nil {
					return m, e
				}
				if dp.id != 0 {
					padding, err = signed(dp)
					if err != nil {
						return m, err
					}
				}
				for _, x := range child.children {
					if x.id == 0xa4 || x.id == 0x75a1 || x.id == 0x8e {
						return m, fmt.Errorf("block transformations unsupported")
					}
				}
			} else if child.id != 0xa3 {
				continue
			}
			b, e := parseBlock(be.data, be.id == 0xa3)
			if e != nil {
				return m, e
			}
			tr, ok := tracks[b.track]
			if !ok {
				return m, fmt.Errorf("undeclared block track")
			}
			if !tr.lacing && len(b.packets) > 1 {
				return m, fmt.Errorf("lacing disabled for track")
			}
			packets += len(b.packets)
			if packets > maxPackets {
				return m, fmt.Errorf("packet count limit")
			}
			if tr.kind != 2 {
				continue
			}
			if b.invisible {
				return m, fmt.Errorf("invisible audio block unsupported")
			}
			if b.time > 0 && clock > uint64(math.MaxInt64-b.time) {
				return m, fmt.Errorf("timestamp overflow")
			}
			tick := int64(clock) + b.time
			if tick > math.MaxInt64/int64(m.scale) || tick < math.MinInt64/int64(m.scale) {
				return m, fmt.Errorf("timestamp overflow")
			}
			b.time = tick * int64(m.scale)
			// Absolute timestamps are bounded too; this makes origin subtraction safe.
			if b.time > 1<<50 || b.time < -(1<<50) {
				return m, fmt.Errorf("absolute timestamp limit")
			}
			b.padding = padding
			m.blocks = append(m.blocks, b)
		}
	}
	if len(m.blocks) == 0 {
		return m, fmt.Errorf("missing audio blocks")
	}
	return m, nil
}
func parseBlock(data []byte, simple bool) (block, error) {
	var b block
	t, n, err := vint(data, false)
	if err != nil || t == 0 || len(data) < n+3 {
		return b, fmt.Errorf("invalid block header")
	}
	b.track = t
	b.time = int64(int16(binary.BigEndian.Uint16(data[n:])))
	flags := data[n+2]
	data = data[n+3:]
	if flags&0x70 != 0 || (!simple && flags&0x81 != 0) {
		return b, fmt.Errorf("unsupported block flags")
	}
	b.invisible = flags&8 != 0
	mode := (flags >> 1) & 3
	if mode == 0 {
		if len(data) == 0 {
			return b, fmt.Errorf("empty block")
		}
		b.packets = [][]byte{data}
		return b, nil
	}
	if len(data) == 0 || data[0] == 0 {
		return b, fmt.Errorf("invalid lace count")
	}
	count := int(data[0]) + 1
	data = data[1:]
	sizes := make([]int, count)
	total := 0
	if mode == 2 {
		if len(data)%count != 0 {
			return b, fmt.Errorf("invalid fixed lace")
		}
		for i := range sizes {
			sizes[i] = len(data) / count
		}
	} else {
		for i := 0; i < count-1; i++ {
			if mode == 1 {
				for {
					if len(data) == 0 {
						return b, fmt.Errorf("truncated Xiph lace")
					}
					v := int(data[0])
					data = data[1:]
					sizes[i] += v
					if sizes[i] > maxLeaf {
						return b, fmt.Errorf("lace size limit")
					}
					if v < 255 {
						break
					}
				}
			} else {
				v, w, e := vint(data, false)
				if e != nil {
					return b, e
				}
				data = data[w:]
				if i == 0 {
					if v > maxLeaf {
						return b, fmt.Errorf("lace size limit")
					}
					sizes[i] = int(v)
				} else {
					delta := int64(v) - int64((uint64(1)<<(7*w-1))-1)
					next := int64(sizes[i-1]) + delta
					if next < 1 || next > maxLeaf {
						return b, fmt.Errorf("invalid EBML lace delta")
					}
					sizes[i] = int(next)
				}
			}
			total += sizes[i]
			if total > maxLeaf {
				return b, fmt.Errorf("lace size limit")
			}
		}
		sizes[count-1] = len(data) - total
	}
	for _, size := range sizes {
		if size < 1 || size > len(data) {
			return b, fmt.Errorf("invalid lace length")
		}
		b.packets = append(b.packets, data[:size])
		data = data[size:]
	}
	if len(data) != 0 {
		return b, fmt.Errorf("lace trailing data")
	}
	return b, nil
}
