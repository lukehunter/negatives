# Camera samples

Real files from each camera in use, for realistic test fixtures. See [docs/testing.md](../../docs/testing.md#test-fixtures).

## Layout

One folder per camera, named after the camera's EXIF `Model` (the same name ingest uses for card folders), keeping the card's own structure inside:

```text
testdata/samples/
└── ILCE-7M4/
    ├── DCIM/100MSDCF/DSC00001.JPG
    ├── DCIM/100MSDCF/DSC00002.ARW
    └── PRIVATE/M4ROOT/CLIP/C0001.MP4      plus its sidecar and index files
```

## What to include per camera

- One JPEG and one raw (or a raw+JPEG pair).
- One short video clip, a few seconds long, with every sidecar and index file the camera writes next to it.
- Anything unusual the camera writes to its cards (settings files, thumbnails, empty files).

## Before committing

- Strip GPS and other personal metadata, so the repo is safe to make public later:
  `exiftool -gps:all= -xmp:geotag= -overwrite_original -r testdata/samples/<camera>`
- Files here are stored with Git LFS (see `.gitattributes` at the repo root). Install it once with `brew install git-lfs && git lfs install`.
