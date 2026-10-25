# Sample manifest

What each sample folder actually contains. Folder names are informal; the camera models come from the files' EXIF data. Each folder is a copy of a real card, structure included.

| Folder | Camera (EXIF `Model`) | Files | Size |
| --- | --- | --- | --- |
| `a7rv` | ILCE-7SM3 **and** ILCE-7RM5 | ARW×7, JPG×7, MP4×2, XML×3, BIN×1, IND×1 | 290 MB |
| `a7s3` | ILCE-7SM3 | ARW×4, JPG×3, MP4×1, XML×2, BIN×1, IND×1 | 131 MB |
| `osmo4p` | PP-041 (DJI Osmo Pocket) | JPG×12, THM×12, SCR×12, DB×1 | 89 MB |
| `sx740hs` | Canon PowerShot SX740 HS | JPG×3, MP4×1, CTG×1 | 22 MB |
| `x100vi` | X100VI | JPG×3, RAF×3, MOV×1 | 210 MB |

## Notes per folder

**`a7s3` and `a7rv`: one card at two points in time.** The card was shot in the a7S III and copied (`a7s3`), then, without being formatted, used in the a7R V and copied again (`a7rv`).

- `a7rv` is a **mixed-camera card**: DSC09304–09308 and clip `s20261025_2206` are from the ILCE-7SM3; DSC09309–09312 and clip `r20261025_2207` are from the ILCE-7RM5.
- Every a7S III file is byte-identical in both copies, except `DATABASE.BIN` and `MEDIAPRO.XML`, which the camera updated when it recorded more. Together the two folders are the "card with old and new files" scenario.
- The earlier copy has a lowercase `private/` folder; the later one has `PRIVATE/`. Paths must therefore be matched case-insensitively (see [docs/schema.md](../../docs/schema.md#conventions)).
- `PRIVATE/SONY/SONYCARD.IND` is a zero-byte file the camera writes on every card.
- Sony video: clips in `M4ROOT/CLIP` with `M01.XML` sidecars, thumbnails in `M4ROOT/THMBNL`, the index `M4ROOT/MEDIAPRO.XML`.

**`osmo4p`**

- Photos in `DCIM/DJI_001` and a 9-frame panorama in `DCIM/PANORAMA/001_0004`. Each photo has a `.THM` thumbnail and a `.SCR` screen preview in a parallel `MISC/THM` tree, plus a camera database in `MISC/`.
- EXIF `Model` is the internal code `PP-041`; the `cameraNames` setting maps it to a readable name.
- GPS tags are present but all zero: location was off. Tools must treat 0,0 placeholders as "no location".
- No video yet; a short clip (location off) would round this out.

**`sx740hs`**: photos and one MP4 in `DCIM/100CANON`, plus Canon's `DCIM/CANONMSC/M3100.CTG` catalog file.

**`x100vi`**: raw+JPEG pairs and one MOV in `DCIM/101_FUJI`, which starts at folder 101 rather than 100.

## Location data

Checked with `exiftool -r -ee3 -gps:all` and by reading JPEG EXIF: no folder contains real location data, only all-zero placeholders. No file has been modified.
