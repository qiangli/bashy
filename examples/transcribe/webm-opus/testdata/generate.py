#!/usr/bin/env python3
"""One-time fixture authoring; NOT a build/test/runtime dependency.
Uses an existing libopus for independent reference PCM and an authored stereo
signal. No inference, encoder download or executable installation. Run with
LIBOPUS=/absolute/path to an existing reviewed development library.
"""
import ctypes as C
import hashlib
import json
import math
import os
from pathlib import Path
import struct
import urllib.request

root = Path(__file__).resolve().parent
lib = C.CDLL(os.environ['LIBOPUS'])
lib.opus_get_version_string.restype = C.c_char_p
lib.opus_decoder_create.argtypes = [C.c_int, C.c_int, C.POINTER(C.c_int)]
lib.opus_decoder_create.restype = C.c_void_p
lib.opus_decode.argtypes = [C.c_void_p, C.c_void_p, C.c_int, C.POINTER(C.c_int16), C.c_int, C.c_int]
lib.opus_decoder_destroy.argtypes = [C.c_void_p]
lib.opus_encoder_create.argtypes = [C.c_int, C.c_int, C.c_int, C.POINTER(C.c_int)]
lib.opus_encoder_create.restype = C.c_void_p
lib.opus_encode.argtypes = [C.c_void_p, C.POINTER(C.c_int16), C.c_int, C.c_void_p, C.c_int]
lib.opus_encoder_destroy.argtypes = [C.c_void_p]

def element(i, *parts):
    payload = b''.join(parts)
    w = 1
    while len(payload) >= (1 << (7*w))-1:
        w += 1
    return i.to_bytes((i.bit_length()+7)//8, 'big') + (len(payload) | (1 << (7*w))).to_bytes(w, 'big') + payload

def uint(i, n):
    return element(i, n.to_bytes(8, 'big'))

def webm(packets, channels, skip, gain=0):
    head = b'OpusHead' + bytes([1, channels]) + struct.pack('<HIhB', skip, 48000, gain, 0)
    track = element(0xae, uint(0xd7, 1), uint(0x73c5, 1), uint(0x83, 2), element(0x86, b'A_OPUS'), element(0x63a2, head), uint(0x56aa, skip*1000000000//48000), element(0xe1, element(0xb5, struct.pack('>d', 48000)), uint(0x9f, channels)))
    blocks = [element(0xa3, b'\x81'+struct.pack('>hB', i*20, 0x80)+p) for i,p in enumerate(packets)]
    return element(0x1a45dfa3, element(0x4282, b'webm'), uint(0x42f7, 1), uint(0x4285, 2)) + element(0x18538067, element(0x1549a966, uint(0x2ad7b1, 1000000)), element(0x1654ae6b, track), element(0x1f43b675, uint(0xe7, 0), *blocks))

def reference(packets, skip):
    err = C.c_int()
    dec = lib.opus_decoder_create(16000, 1, C.byref(err))
    assert dec and err.value == 0
    out = bytearray()
    for p in packets:
        pcm = (C.c_int16*1920)()
        n = lib.opus_decode(dec, p, len(p), pcm, 1920, 0)
        assert n == 320, n
        out += struct.pack('<'+'h'*n, *pcm[:n])
    lib.opus_decoder_destroy(dec)
    return out[2*((skip+1)//3):]

url = 'https://upload.wikimedia.org/wikipedia/commons/2/2f/Wikispeech-sample2.opus'
source = urllib.request.urlopen(urllib.request.Request(url, headers={'User-Agent': 'curl/8.7.1'}), timeout=15).read(100000)
assert hashlib.sha1(source).hexdigest() == 'c8e4fd2be6992f0764546cd5fb3d1fc03702e4aa'
packets = []
partial = bytearray()
pos = 0
while pos < len(source):
    assert source[pos:pos+4] == b'OggS'
    n = source[pos+26]
    lengths = source[pos+27:pos+27+n]
    pos += 27+n
    for n in lengths:
        partial += source[pos:pos+n]
        pos += n
        if n < 255:
            packets.append(bytes(partial))
            partial.clear()
head = packets[0]
assert head[:8] == b'OpusHead' and head[9] == 1
skip = struct.unpack_from('<H', head, 10)[0]
speech = packets[2:102]
(root/'speech.webm').write_bytes(webm(speech, 1, skip))
(root/'speech-reference.pcm').write_bytes(reference(speech, skip))
# Remux the complete CC0 speech for the native end-to-end transcription gate.
# The original Opus packets and pre-skip are unchanged.
long_speech = packets[2:]
(root/'speech-long.webm').write_bytes(webm(long_speech, 1, skip))

err = C.c_int()
enc = lib.opus_encoder_create(48000, 2, 2049, C.byref(err))
assert enc and err.value == 0
stereo = []
for frame in range(30):
    pcm = (C.c_int16*1920)()
    for i in range(960):
        at = (frame*960+i)/48000
        pcm[2*i] = round(9000*math.sin(2*math.pi*440*at))
        pcm[2*i+1] = round(5000*math.sin(2*math.pi*880*at))
    buf = C.create_string_buffer(4000)
    n = lib.opus_encode(enc, pcm, 960, buf, len(buf))
    assert n > 0
    stereo.append(buf.raw[:n])
lib.opus_encoder_destroy(enc)
(root/'stereo.webm').write_bytes(webm(stereo, 2, 312))
(root/'stereo-reference.pcm').write_bytes(reference(stereo, 312))
manifest = {'speech_source': url, 'source_sha256': hashlib.sha256(source).hexdigest(), 'speech_packets': len(speech), 'long_speech_packets': len(long_speech), 'long_speech_packet_span_ms': len(long_speech) * 20, 'speech_preskip': skip, 'reference_library': lib.opus_get_version_string().decode(), 'reference_library_sha256': hashlib.sha256(Path(os.environ['LIBOPUS']).read_bytes()).hexdigest(), 'files': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(root.iterdir()) if p.suffix in ('.webm', '.pcm')}}
(root/'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
print(json.dumps(manifest, indent=2))
