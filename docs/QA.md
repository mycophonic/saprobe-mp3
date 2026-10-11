# Testing MP3

## Conformance (`TestMP3Decode`)

Cross-decoder comparison tests across all supported sample rate, channel count, and bitrate combinations. Subtests are named `{sampleRate}Hz_{channels}ch/{bitrateMode}`.

```bash
# Full suite (all tools)
just test

# Conformance only
go test ./tests/ -run TestMP3Decode -count=1 -v

# Single subtest
go test ./tests/ -run TestMP3Decode/44100Hz_2ch/cbr320 -count=1 -v
```

**Test matrix:**

- **Sample rates:** 16000, 22050, 24000 (MPEG 2), 32000, 44100, 48000 (MPEG 1)
- **Channels:** 1, 2
- **Bitrates (MPEG 1, >= 32kHz):** CBR 128k, CBR 320k, VBR q2
- **Bitrates (MPEG 2, < 32kHz):** CBR 96k, CBR 160k, VBR q2

Total: 6 sample rates x 2 channels x 3 bitrates = 36 subtests (with lame: same count, cross-compared against 3 decoders).

Each file is encoded with ffmpeg (libmp3lame), then decoded by all available decoders. Outputs are cross-compared using lossy sample comparison (MP3 is lossy; decoders may differ by a few LSBs due to floating-point rounding).

**Verification:**

- Saprobe always outputs stereo (mono input is duplicated to both channels). For mono sources, ffmpeg and lame outputs are duplicated to stereo before comparison.
- Format metadata verified for saprobe: sample rate, bit depth (always 16), channels (always 2).
- Cross-decoder comparison uses `CompareLossySamples` which tolerates minor differences inherent to MP3 decoding.

**Reference tools:**

| Tool   | Role   | Notes                                     |
|--------|--------|-------------------------------------------|
| ffmpeg | Decode | libavcodec MP3 decoder                    |
| lame   | Decode | Reference LAME decoder (`--decode`)       |

Both tools are optional; tests skip gracefully when unavailable.

## Benchmarks

### Synthetic Decode (`TestMP3BenchmarkDecode`)

Benchmarks decoding across all available decoders using synthetic white noise encoded via ffmpeg. Skipped in `-short` mode.

```bash
go test ./tests/ -run TestMP3BenchmarkDecode -count=1 -v
```

- **Configs:** 44.1kHz stereo CBR 320k, CBR 128k, VBR q2
- **Duration:** 10 seconds per config
- **Decoders:** saprobe, ffmpeg, lame, mpg123, coreaudio (macOS only)
- **Iterations:** 10 per configuration
- **Statistics:** median, mean, stddev, min, max

### Real Files (`TestMP3BenchmarkDecodeFile`)

Benchmarks decoding natural MP3 files. 10 iterations, same decoder set and statistics as synthetic benchmarks. Skipped in `-short` mode.

```bash
BENCH_MP3_FILE='/path/to/file.mp3' go test ./tests/ -run TestMP3BenchmarkDecodeFile -count=1 -v
```

Reference files selected for variety in duration, bitrate, and content type.
All 44.1kHz stereo.
Decode times are median over 10 iterations.
Ratio columns show saprobe time relative to each reference tool (>1x = saprobe slower, <1x = saprobe faster).

| File | Bitrate | Duration | Size | saprobe | ffmpeg | vs ffmpeg | lame | vs lame | mpg123 | vs mpg123 | coreaudio | vs coreaudio | Character |
|------|---------|----------|------|---------|--------|-----------|------|---------|--------|-----------|-----------|--------------|-----------|
| Dolphy — The Theme | 320k | 0:17 | 0.6 MB | 94ms | 84ms | 1.1x | 32ms | 2.9x | 18ms | 5.2x | 26ms | 3.6x | Very short, live jazz |
| Waldron — At Kamenoi Besso | 192k | 4:29 | 6.2 MB | 1.350s | 389ms | 3.5x | 413ms | 3.3x | 222ms | 6.1x | 361ms | 3.7x | Mid-length, sparse piano trio |
| Mingus — Peggy's Blue Skylight | 320k | 19:04 | 43.6 MB | 6.188s | 1.529s | 4.0x | 2.112s | 2.9x | 992ms | 6.2x | 1.608s | 3.8x | Long, dense jazz workshop |
| Zorn — Kitchen Concert | 128k | 10:55 | 10.0 MB | 3.043s | 725ms | 4.2x | 802ms | 3.8x | 399ms | 7.6x | 766ms | 4.0x | Mid-length, experimental |

On short files (<30s), saprobe approaches ffmpeg (1.1x) due to zero process-spawn overhead.
On longer files, mpg123 is the fastest (5-8x faster than saprobe), followed by ffmpeg and lame (~3-4x).
CoreAudio (CGO, in-process) is ~3.6-4.0x faster. The saprobe decoder is pure Go with no SIMD.

Full paths:

```
/Volumes/Anisotope/Music/Processor/Lossy/[LOSSY]/Eric Dolphy/1961 - Live in Germany/05-The Theme.mp3
/Volumes/Anisotope/Music/Processor/Lossy/[LOSSY]/Mal Waldron/02 Live Albums/1995 - Travellin' in Soul-Time (with Jeanne Lee, Toru Tenda) [192kbps]/07 - At Kamenoi Besso.mp3
/Volumes/Anisotope/Music/Processor/Lossy/[LOSSY]/Charles Mingus - Collection/1964 - 1965.Charles Mingus - The Jazz Workshop Concerts (7CD) [320]/CD2/01. Peggy's Blue Skylight (A).mp3
/Volumes/Anisotope/Music/Baal/need.mb.data.net/John Zorn/1981 - Kitchen Concert - 128/kitchenconcert - 04.mp3
```

#### CPU Profile

```bash
hack/bench.sh TestMP3BenchmarkDecodeFile '/path/to/file.mp3'
```

## Mass testing

Comparative decoding is being done on a set of 8515 MP3 files from the Anisotope collection. Distribution is predominantly 44.1kHz stereo CBR 320k with some 192k and 128k files.
