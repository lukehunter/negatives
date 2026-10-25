# Camera samples

Real files from each camera in use, for realistic test fixtures. See [docs/testing.md](../../docs/testing.md#test-fixtures).

## Layout

One folder per card copy, with any informal name, keeping the card's own structure inside. [MANIFEST.md](MANIFEST.md) records what each folder really contains: camera models, notable files and quirks. Update it whenever a sample is added.

```text
testdata/samples/
└── a7rv/
    ├── DCIM/100MSDCF/DSC09310.ARW
    └── PRIVATE/M4ROOT/CLIP/r20261025_2207.MP4      plus its sidecar and index files
```

## What to include per camera

- One JPEG and one raw (or a raw+JPEG pair).
- One short video clip, a few seconds long, with every sidecar and index file the camera writes next to it.
- Anything unusual the camera writes to its cards (settings files, thumbnails, empty files).

## Before committing

- Check for location data, so the repo is safe to make public later: `exiftool -r -ee3 -a -G1 -gps:all testdata/samples | grep -i gps`. All-zero values are placeholders and fine. Prefer re-shooting with location off over stripping, so samples stay unmodified.
- Files here are stored with Git LFS (see `.gitattributes` at the repo root). Install it once with `brew install git-lfs && git lfs install`.
