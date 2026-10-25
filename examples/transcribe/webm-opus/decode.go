package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/pion/opus"
)

// The codec is synchronous. A defensive boundary converts codec panics on
// adversarial packets into ordinary failures; it cannot hide partial success.
func decodePacket(d *opus.Decoder, packet []byte, pcm []float32) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			n = 0
			err = fmt.Errorf("invalid Opus packet (codec panic)")
		}
	}()
	if len(packet) == 0 || len(packet) > 65536 {
		return 0, fmt.Errorf("Opus packet size limit")
	}
	return d.DecodeToFloat32(packet, pcm)
}
func decode(ctx context.Context, m media, out io.Writer) (int64, error) {
	return decodeLimited(ctx, m, out, maxOutput/2)
}

func decodeLimited(ctx context.Context, m media, out io.Writer, limit int64) (int64, error) {
	d, err := opus.NewDecoderWithOutput(16000, 1)
	if err != nil {
		return 0, err
	}
	scratch := make([]float32, 1920)
	// Round all trims to the nearest output sample (one sample = 62,500 ns).
	skip := (int64(m.track.skip) + 1) / 3
	var raw, written int64
	origin := m.blocks[0].time
	for _, b := range m.blocks {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		if b.time < origin || b.time-origin > int64(maxOutput/2)*62500 {
			return 0, fmt.Errorf("timestamp outside output limit")
		}
		drift := (b.time - origin) - raw*62500
		tolerance := int64(m.scale) + 62500
		if drift < -tolerance || drift > tolerance {
			return 0, fmt.Errorf("discontinuous audio timestamps")
		}
		samples := make([]float32, 0, len(b.packets)*320)
		for _, packet := range b.packets {
			if err = ctx.Err(); err != nil {
				return 0, err
			}
			n, e := decodePacket(&d, packet, scratch)
			if e != nil {
				return 0, fmt.Errorf("Opus decode: %w", e)
			}
			if n < 1 || n > len(scratch) {
				return 0, fmt.Errorf("invalid decoded sample count")
			}
			raw += int64(n)
			if raw > limit {
				return 0, fmt.Errorf("decoded output limit")
			}
			samples = append(samples, scratch[:n]...)
		}
		// Signed DiscardPadding applies to the whole BlockGroup, including laces.
		// Positive trims the end, negative trims the beginning. Check before abs.
		if b.padding > int64(len(samples))*62500 || b.padding < -int64(len(samples))*62500 {
			return 0, fmt.Errorf("DiscardPadding exceeds block duration")
		}
		left, right := int64(0), int64(len(samples))
		if b.padding < 0 {
			left = (-b.padding + 31250) / 62500
		} else {
			right -= (b.padding + 31250) / 62500
		}
		consumed := int64(len(samples))
		initial := skip
		if initial > consumed {
			initial = consumed
		}
		skip -= initial
		// Pre-skip and negative block padding are independent cuts on the same
		// decoded timeline; their union must not discard the same samples twice.
		if initial > left {
			left = initial
		}
		if left > right {
			left = right
		}
		buf := make([]byte, (right-left)*2)
		for i, s := range samples[left:right] {
			v := float64(s) * m.track.gain
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return 0, fmt.Errorf("nonfinite decoded sample")
			}
			v = math.Round(v * 32768)
			if v > 32767 {
				v = 32767
			}
			if v < -32768 {
				v = -32768
			}
			binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(v)))
		}
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		if _, err = out.Write(buf); err != nil {
			return 0, err
		}
		written += right - left
	}
	if written == 0 || skip > 0 {
		return 0, fmt.Errorf("empty audio after trimming")
	}
	return written, nil
}
func wavHeader(frames int64) []byte {
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
	return b
}
