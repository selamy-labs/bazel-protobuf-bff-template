"""Assembles the local demo's frames into one small GIF (spec 012 FR-024), and writes the record beside it:
the application revision the frames were captured at, the command, the frame captions, the size and the digest.
Run through the pinned uv Python with the hash-locked Pillow; never through the host's Python.

    assemble.py <frames directory> <output gif> <output json> <revision>
"""
import hashlib, json, os, pathlib, sys

from PIL import Image

frames_dir, gif_path, json_path, revision = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), pathlib.Path(sys.argv[3]), sys.argv[4]
files = sorted(p for p in frames_dir.iterdir() if p.suffix == ".png")
if len(files) < 2:
    sys.exit("assemble: fewer than two frames in %s" % frames_dir)
WIDTH = 640
frames = []
for file in files:
    image = Image.open(file).convert("RGB")
    height = round(image.height * WIDTH / image.width)
    frames.append(image.resize((WIDTH, height), Image.LANCZOS).quantize(colors=128, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE))
# Each step is shown long enough to read; the last frame holds longer before the loop restarts.
durations = [1400] * len(frames)
durations[-1] = 3000
gif_path.parent.mkdir(parents=True, exist_ok=True)
frames[0].save(gif_path, save_all=True, append_images=frames[1:], duration=durations, loop=0, optimize=True, disposal=1)
data = gif_path.read_bytes()
LIMIT = 1_500_000
if len(data) > LIMIT:
    sys.exit("assemble: %s is %d bytes, over the %d-byte bound" % (gif_path, len(data), LIMIT))
record = {
    "label": "LOCAL demo: the real page against the real local server, recorded from the one command; no hosted claim",
    "revision": revision,
    "command": "tools/dev/record.sh",
    "frames": [f.stem for f in files],
    "width": WIDTH,
    "height": frames[0].height,
    "bytes": len(data),
    "sha256": hashlib.sha256(data).hexdigest(),
    "encoder": "pillow, pinned by hash in tools/dev/record/requirements.lock",
}
json_path.write_text(json.dumps(record, indent=2) + "\n")
print("assemble: %s, %d frames, %d bytes" % (gif_path, len(frames), len(data)))
