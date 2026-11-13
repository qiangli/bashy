#!/usr/bin/env python3
"""Build a Replicate model input from a bashy-svc-request-v1 file.

Media data stays in a 0600 JSON file; it is never passed in argv or logged.
"""
import base64
import json
import pathlib
import sys


def fail(message):
    raise ValueError(message)


def required(request, name):
    value = request.get(name)
    if not isinstance(value, str) or not value:
        fail(f"{name} is required")
    return value


def local_file(request_path, name, value):
    path = pathlib.Path(value)
    if not path.is_absolute():
        path = request_path.parent / path
    if not path.is_file():
        fail(f"{name} file is missing")
    return path


def data_url(path):
    mime = {
        ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
        ".webp": "image/webp", ".wav": "audio/wav", ".mp3": "audio/mpeg",
        ".m4a": "audio/mp4",
    }.get(path.suffix.lower())
    if not mime:
        fail("input must be png, jpeg, webp, wav, mp3 or m4a")
    if path.stat().st_size > 12_000_000:
        fail("input exceeds 12 MB data URL limit; use a smaller file")
    return "data:" + mime + ";base64," + base64.b64encode(path.read_bytes()).decode("ascii")


def build(kind, role, request_path, request):
    if request.get("schema_version") != "bashy-svc-request-v1":
        fail("unsupported schema_version")
    if kind == "song":
        if request.get("instrumental") is True:
            fail("this vocal song model does not support instrumental=true")
        lyrics = request.get("lyrics")
        if not lyrics:
            lyrics = local_file(request_path, "lyrics_file", required(request, "lyrics_file")).read_text()
        if not lyrics:
            fail("lyrics or lyrics_file is required")
        return "song", {"prompt": required(request, "prompt"), "lyrics": lyrics, "audio_format": "wav"}
    if kind == "image":
        prompt = required(request, "prompt")
        count = request.get("count", 1)
        if type(count) is not int or not 1 <= count <= 4:
            fail("count must be 1-4")
        if role == "image.edit":
            if request.get("mask"):
                fail("the API edit model does not support a mask")
            if count != 1:
                fail("image.edit supports one output")
            source = local_file(request_path, "image", required(request, "image"))
            return "image-edit", {"prompt": prompt, "input_image": data_url(source), "output_format": "png"}
        refs = request.get("reference_images") or []
        if refs:
            if not isinstance(refs, list) or len(refs) != 1 or count != 1 or not isinstance(refs[0], str):
                fail("reference generation supports one reference and one output")
            source = local_file(request_path, "reference_images", refs[0])
            return "image-edit", {"prompt": prompt, "input_image": data_url(source), "output_format": "png"}
        negative = request.get("negative")
        if negative:
            prompt += ". Avoid: " + str(negative)
        return "image", {"prompt": prompt, "num_outputs": count, "output_format": "png"}
    if kind == "animate":
        duration = request.get("duration_s", 0)
        if type(duration) is not int or not 0 <= duration <= 15:
            fail("duration_s must be an integer from 0 to 15; split longer audio")
        image = local_file(request_path, "image", required(request, "image"))
        audio = local_file(request_path, "audio", required(request, "audio"))
        return "animate", {"image": data_url(image), "audio": data_url(audio)}
    fail("unknown role")


def main():
    kind, role, request_name, output_name = sys.argv[1:]
    request_path = pathlib.Path(request_name)
    request = json.loads(request_path.read_text())
    command, model_input = build(kind, role, request_path, request)
    output = pathlib.Path(output_name)
    output.write_text(json.dumps(model_input, separators=(",", ":")))
    output.chmod(0o600)
    print(command)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print("replicate-media: " + str(exc), file=sys.stderr)
        sys.exit(2)
