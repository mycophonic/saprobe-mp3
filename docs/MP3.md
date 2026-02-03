# MP3 Decoder Behavior

## Decoder delay and gapless playback

Every MP3 decoder introduces a 529-sample delay from the synthesis filterbank
priming. This delay is an inherent property of the MP3 format, not an encoding
artifact.

### How different decoders handle the delay

| Decoder | Trims decoder delay | Source of trim info |
|---------|--------------------|--------------------|
| saprobe | Yes, always | Hard-coded 529 + LAME gapless if present |
| lame | Yes, always | Hard-coded 529 + LAME gapless if present |
| ffmpeg | Only with gapless info | LAME gapless tag in XING/Info frame |
| mpg123 | Only with gapless info | LAME gapless tag in XING/Info frame |

For files **with** LAME gapless metadata (encoder delay + padding stored in
the XING/Info frame), all four decoders produce identical output lengths.

For files **without** gapless metadata, ffmpeg and mpg123 output 529 extra
stereo samples (2116 bytes at 16-bit stereo) at the start of the stream.
saprobe and lame still trim the 529-sample decoder delay unconditionally,
producing shorter output. This is the correct behavior for gapless playback:
those leading samples are filterbank transient, not audio content.

### saprobe's trimming logic

Start trim (leading samples removed):

    start = lame_encoder_delay + 529
    if xing_tag_present:
        start += samples_per_frame

End trim (trailing samples removed):

    end = max(lame_padding - 529, 0)

When no LAME gapless info exists, `lame_encoder_delay` and `lame_padding` are
both zero, so only the 529-sample decoder delay is trimmed.

The extra `samples_per_frame` added when an XING tag is present accounts for
the XING/Info frame itself, which is a silent frame that encoders insert at the
start of the stream to carry metadata. It occupies one frame slot but contains
no audio.

### XING/Info frame parsing

saprobe reads the first frame's side info to locate the XING or Info tag.
If found, it extracts:

- Frame count and byte count (for duration/seeking)
- TOC (table of contents, for seeking)
- LAME tag at a fixed offset: 3 bytes encoding a 12-bit encoder delay and
  12-bit padding value

This matches the de facto standard established by the LAME encoder and honored
by all major decoders.

## Bit reservoir and frame skipping

MP3 uses a bit reservoir where a frame's main audio data can reference bytes
from previous frames via the `main_data_begin` pointer in side info. When the
decoder has not accumulated enough reservoir data (e.g. at the start of a
stream, or after sync recovery), it cannot decode the current frame.

saprobe handles this by:

1. Consuming the frame's main data into the reservoir (so future frames can
   reference it)
2. Outputting silence for the current frame (zeroed PCM buffer)
3. Continuing to the next frame

This avoids producing garbage audio while still building up the reservoir for
subsequent frames. The first few frames of any MP3 stream typically require
this treatment.

## Sync recovery and corrupt streams

When saprobe encounters bytes that don't form a valid frame header, it enters
a sync search: scanning byte-by-byte for the 11-bit sync word (0xFFE0),
bounded at 128 KB to prevent unbounded scanning on severely corrupt files.

When sync recovery skips bytes, all stateful buffers are reset:

- **Bit reservoir**: cleared to prevent stale data from corrupting future frames
- **IMDCT overlap-add store**: zeroed to prevent cross-fade artifacts
- **Synthesis filterbank FIFO**: zeroed to prevent ringing from pre-corruption
  state

This means recovery from corruption is clean but introduces a brief silence
(the frames needed to re-prime the filterbank and reservoir).

## Known discrepancies with reference decoders

### Embedded ID3v1 tags mid-stream

Some files (observed in the Supersmell album, circa 2000) contain ID3v1 tags
embedded in the middle of the audio stream, likely from a faulty CD ripping
tool that concatenated files incorrectly. The ID3v1 block is 128 bytes of
ASCII metadata followed by variable amounts of zero-padding.

Decoder behavior on these files:

| Decoder | Behavior |
|---------|----------|
| ffmpeg | Scans through the junk, recovers sync, decodes full stream |
| saprobe | Scans through the junk (up to 128 KB), recovers sync, decodes full stream |
| lame | Often fails entirely (exits with error, produces 0 bytes) |
| mpg123 | Stops decoding at the embedded tag (produces only frames before the tag) |

These files are genuinely corrupt. There is no "correct" behavior: the
decoders that recover and the decoders that stop are both making reasonable
choices. saprobe's approach (scan and recover) matches ffmpeg, which is the
most widely deployed decoder.

### Sample-level rounding (max diff = 1 LSB)

ffmpeg and mpg123 produce output of identical length but differ by at most
1 LSB (~50% of samples). This is a floating-point rounding difference in
the synthesis filterbank: one rounds up, the other rounds down. Both outputs
are equally valid.

saprobe and lame also differ by at most 1-2 LSB from each other and from
the ffmpeg/mpg123 pair after accounting for the decoder delay offset. This
is within the expected tolerance for independent MP3 decoder implementations.

### Length differences without gapless info (2116 bytes)

As described above, saprobe and lame produce output that is exactly 2116
bytes shorter than ffmpeg and mpg123 on files without LAME gapless metadata.
This is `529 samples * 2 channels * 2 bytes = 2116` bytes: the decoder
delay that saprobe and lame trim unconditionally.

### Length differences with different LAME padding (6724 bytes)

A small number of files show a 6724-byte difference between ffmpeg/mpg123
and lame. This is `529 * 2 + 1152 * 2 = 6724` bytes (in stereo 16-bit),
corresponding to the decoder delay plus one full frame. This occurs when
lame's decoder interprets the LAME padding field differently or when the
XING frame handling differs between decoders.

## Mass test failure log

Results from decoding the test corpus with four decoders (saprobe, ffmpeg,
lame, mpg123). Classifications:

- **FAIL**: references agree, saprobe differs. Indicates a saprobe bug.
- **HARD CASE**: references themselves disagree. No single correct answer.

### FAILs (saprobe bugs) — FIXED

All three failures were caused by two bugs, both now fixed:

**Bug 1: Reservoir replacement instead of accumulation** (`maindata.go`).
When `main_data_begin > reservoir`, saprobe replaced the reservoir with only
the current frame's data (`res.len = mainDataSize`). mpg123 accumulates
instead (`res.len += mainDataSize`). The replacement caused cascading
insufficient-reservoir frames. Fixed by appending new data after existing
reservoir content.

**Bug 2: SkipDecode frames were silently dropped** (`decode.go`).
Frames with insufficient reservoir were skipped entirely (`continue` in the
read loop), producing fewer output samples. mpg123 outputs silence (zeroed
PCM) for these frames, maintaining correct output length. Fixed by emitting
a zeroed buffer instead of skipping.

**Bug 3: `skipTags` used `Seek(SeekCurrent)` incompatible with buffered readers**
(`decode.go`). The `skipTags` function used `Seek(-3, SeekCurrent)` to undo a
3-byte probe read. With `filesystem.ReadSeeker` (4096-byte buffered reader),
`SeekCurrent` references the underlying file position (4096) rather than the
logical position (3), causing the decoder to start ~4KB into the file and lose
~10 frames. Fixed by using absolute seeks (`SeekStart`) throughout `skipTags`.

| # | File | Old sp | ff | lm | mp | Old deficit | Status |
|---|------|--------|----|----|----|-------------|--------|
| 119 | `jazz.lossless/Taylor, Cecil/.../03-05 Enter, Evening (Alternate Take).mp3` | 107802044 | 107900928 | N/A | 107900928 | 98884 B = 21.5 frames | FIXED. Now sp=107898812, ff-sp=2116 (decoder delay only). |
| 124 | `perso/Fabin/Le Lac au vin d'or.mp3` | 38382524 | 38435328 | 38433212 | 38435328 | 52804 B = 11.5 frames | FIXED. Now sp=38433212, sp=lm, ff-sp=2116. |
| 125 | `perso/Fabin/Petite dame Belle.mp3` | 42096572 | 42149376 | 42147260 | 42149376 | 52804 B = 11.5 frames | FIXED. Now sp=42147260, sp=lm, ff-sp=2116. |

### HARD CASEs (references disagree)

#### Supersmell album (embedded ID3v1 mid-stream)

All 13 tracks from this album contain ID3v1 tags spliced into the middle of
the audio stream. mpg123 stops at the tag, lame often fails entirely, ffmpeg
and saprobe scan through and recover. No action needed.

| # | File |
|---|------|
| 2 | `2000-08-07 - Supersmell/.../01-15 Psychoflute.mp3` |
| 3 | `2000-08-07 - Supersmell/.../02-15 Tijuana 65.mp3` |
| 4 | `2000-08-07 - Supersmell/.../03-15 Dreamtime in Memphis.mp3` |
| 5 | `2000-08-07 - Supersmell/.../04-15 Erotic City.mp3` |
| 6 | `2000-08-07 - Supersmell/.../05-15 Baby Elephant Walk.mp3` |
| 7 | `2000-08-07 - Supersmell/.../06-15 The Cape and the Crown (Sha La La).mp3` |
| 8 | `2000-08-07 - Supersmell/.../07-15 One More Try.mp3` |
| 9 | `2000-08-07 - Supersmell/.../08-15 Under Control.mp3` |
| 10 | `2000-08-07 - Supersmell/.../09-15 No. 9.mp3` |
| 11 | `2000-08-07 - Supersmell/.../10-15 Meeting Isaac (For Your Love).mp3` |
| 12 | `2000-08-07 - Supersmell/.../11-15 Glass Tiger.mp3` |
| 13 | `2000-08-07 - Supersmell/.../12-15 Riders on the Storm.mp3` |
| 14 | `2000-08-07 - Supersmell/.../13-15 Draw Me Something, Mr. Jack Brewer.mp3` |

#### Other HARD CASEs

| # | File | sp | ff | lm | mp | Notes |
|---|------|----|----|----|----|-------|
| 118 | `jazz.lossless/Brown, Clifford & Roach, Max/.../13-13 - Never Leave Me.mp3` | 72242108 | 72244224 | 72246716 | 72248832 | All four produce different lengths. sp=ff+529 (normal delay trim), but lm=mp+529 suggesting lm and mp also disagree on gapless trim. ff is the odd one out among references. No action needed. |
| 139 | `sweep.zorn/Ambitious Lovers/1988-09-21 - Greed/.../10-13 Para Nao Contrariar Voce.mp3` | 27954620 | 27947520 | 27952316 | 27958272 | Total disagreement: all four decoders produce different lengths. Diffs are fractional frames (not multiples of 1152). Likely a corrupt or unusual file. No action needed. |
