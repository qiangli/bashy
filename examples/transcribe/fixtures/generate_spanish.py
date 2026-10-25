#!/usr/bin/env python3
"""Author the bounded CC0 Spanish WAV fixture; never used by the native gate."""

import hashlib
import io
from pathlib import Path
import urllib.request
import wave

URL = ('https://upload.wikimedia.org/wikipedia/commons/6/63/'
       'LL-Q1321_%28spa%29-Rodrigo5260-de_todo_hay_en_la_vi%C3%B1a_del_Se%C3%B1or.wav')
SOURCE_SHA256 = 'bf2168307813beb1037c3ee4e9efceee62bfbd0e4920adcc09f74d03e4b5118b'
SOURCE_BYTES = 215852


def main():
    request = urllib.request.Request(URL, headers={
        'User-Agent': 'S503-controlled-fixture/1.0 (github.com/qiangli/bashy)'})
    with urllib.request.urlopen(request, timeout=30) as response:
        source = response.read(SOURCE_BYTES + 1)
    if len(source) != SOURCE_BYTES or hashlib.sha256(source).hexdigest() != SOURCE_SHA256:
        raise SystemExit('Spanish CC0 source bytes differ from reviewed Wikimedia file')
    with wave.open(io.BytesIO(source), 'rb') as wav:
        params = wav.getparams()
        if (params.nchannels, params.sampwidth, params.framerate, params.nframes,
                params.comptype) != (1, 2, 48000, 107904, 'NONE'):
            raise SystemExit('Spanish source WAV format changed')
        speech = wav.readframes(params.nframes)
    # Repeating the same spoken proverb with a short silence makes automatic
    # language detection and subtitle timing measurable with the tiny model.
    silence = b'\0' * (48000 * 2 // 2)  # 500 ms PCM16 mono
    target = Path(__file__).with_name('spanish-proverb.wav')
    with wave.open(str(target), 'wb') as wav:
        wav.setnchannels(1)
        wav.setsampwidth(2)
        wav.setframerate(48000)
        wav.writeframes((speech + silence) * 2 + speech)
    print(target, target.stat().st_size, hashlib.sha256(target.read_bytes()).hexdigest())


if __name__ == '__main__':
    main()
