# MP3 Decoders: Prevalence and Technical Specifications

## Overview

MP3 decoding is a solved problem. The format is fully specified (ISO/IEC 11172-3, ISO/IEC 13818-3), all patents expired in April 2017, and conformance test vectors exist. Unlike encoding — where quality varies wildly between implementations — decoding has a hard correctness target: reproduce the reference output within specified tolerances.

That said, implementations still differ in speed, accuracy, robustness to malformed streams, memory footprint, and platform support.

---

## Decoder Comparison Table

| Rank | Decoder | MPEG Support | Accuracy | Speed | Size | Platform | Notes |
|------|---------|-------------|----------|-------|------|----------|-------|
| 1 | **mpg123 (libmpg123)** | 1.0, 2.0, 2.5 — Layers I, II, III | ISO compliant (float), near-compliant (int) | Fastest | ~118 KiB (stripped) | Cross-platform | Best overall. Actively maintained. LGPL 2.1. |
| 2 | **FFmpeg (libavcodec)** | 1.0, 2.0, 2.5 — Layers I, II, III | ISO compliant | Very fast | Part of libavcodec (~30 MB total) | Cross-platform | Powers VLC, mpv, Kodi, Plex, Jellyfin, most media software. LGPL/GPL. |
| 3 | **minimp3 (lieff)** | 1.0, 2.0, 2.5 — Layers I, II, III | ISO compliant (PSNR > 96 dB) | Very fast (SSE/NEON) | Single header (~30 KB source) | Cross-platform | Public domain. Ideal for embedding. Clean-room from FFmpeg's decoder logic. |
| 4 | **MAD (libmad)** | 1.0, 2.0, 2.5 — Layers I, II, III | 24-bit fixed-point (very high precision) | Moderate | ~150 KiB | Cross-platform | **Dead since 2004.** No updates in 20+ years. GPL. Still used in legacy codebases. |
| 5 | **Fraunhofer IIS** | 1.0, 2.0, 2.5 — Layers I, II, III | Reference quality | Fast | Proprietary | Embedded in apps/OS | Not standalone. Built into various commercial software/hardware. |
| 6 | **Apple CoreAudio** | 1.0, 2.0, 2.5 — Layer III | High | Fast | Part of OS | macOS, iOS | System framework. Used by Apple Music, iTunes, QuickTime. |
| 7 | **Windows Media Foundation** | 1.0, 2.0, 2.5 — Layers I, II, III | High | Fast | Part of OS | Windows | Native OS codec. Used by Windows Media Player, Edge, apps. |
| 8 | **Rockbox** | 1.0, 2.0, 2.5 — Layers I, II, III | High | Optimized for ARM | Minimal | Embedded/portable | Uses libmad-derived code with ARM assembly optimizations. |

---

## Historical / Obsolete Decoders

| Decoder | Era | Notes |
|---------|-----|-------|
| **Fraunhofer WinPlay3** | 1995 | First real-time MP3 player. Required 486DX4/100. Historical milestone. |
| **Fraunhofer l3dec** | 1994 | First public MP3 decoder software. Command-line. |
| **Xing decoder** | Late 1990s | Very fast for the era but poor accuracy. |
| **KeyJ's minimp3** | 2007 | Original minimp3. Failed ISO conformance tests. Superseded by lieff/minimp3. Not the same project. |

---

## Detailed Notes

### mpg123 (Rank 1)
- **Usage**: Default MP3 decoder for most Linux distributions, MPD, many audio players, libsndfile (since 1.1.0).
- **Speed**: Approximately 2x faster than libmad. A Core2Duo can do 500x real-time decoding. ARM optimized. x86/x86-64 SIMD (SSE, AVX).
- **Accuracy**: Floating-point output passes ISO compliance. Integer output passes with `--enable-int-quality` (default since ~2012). Dithering available for 16-bit output.
- **Gapless**: Full gapless playback support via LAME Info/Xing headers.
- **Features**: Sample-accurate seeking, all Layers (I/II/III), free-format bitrate support, downsampling (2:1, 4:1), streaming support.
- **Maintenance**: Actively maintained by Thomas Orgis. Regular releases. Latest: 1.33.x (2025+).
- **License**: LGPL 2.1.

### FFmpeg (Rank 2)
- **Usage**: The backbone of nearly all media software. If your app plays MP3 and isn't using mpg123 or a platform codec, it's almost certainly using FFmpeg's decoder.
- **Implementation**: Native (not a wrapper). Independent implementation in libavcodec.
- **Quality**: ISO compliant. No known correctness issues with well-formed streams.
- **Note**: The minimp3 (lieff) decoder is actually derived from FFmpeg's MP3 decoder code, cleaned up into a single header.

### minimp3 — lieff (Rank 3)
- **Usage**: Embedded applications, game engines, single-file projects. Used by Ardour (briefly), various indie game engines.
- **Key feature**: Single-header library. `#include "minimp3.h"` and you have a complete MP3 decoder. Zero dependencies.
- **Quality**: ISO conformant (PSNR > 96 dB on all test vectors). Free-format support.
- **History**: Created because the original KeyJ minimp3 (2007) failed conformance tests. lieff/minimp3 is a clean-room rewrite derived from FFmpeg's decoder. Public domain (CC0).
- **SIMD**: SSE and NEON support. Competitive speed with mpg123 and FFmpeg.

### MAD — libmad (Rank 4)
- **Usage**: Legacy. Still found in older MPD configurations, some embedded Linux setups, foobar2000 (optional).
- **Quality**: Uses 32-bit fixed-point throughout (no floating-point). Exceptionally high precision — 24-bit output accuracy. Was considered the gold standard for accuracy in the 2000s.
- **Problem**: **Dead.** Last release: 0.15.1b (February 2004). 20+ years without updates. Known issues:
    - Produces low-level noise in silent passages
    - Some stream handling bugs with VBR
    - No SIMD optimizations for modern CPUs
- **Recommendation**: Replace with mpg123 in all new projects. There is zero reason to use libmad in 2025.
- **License**: GPL 2 (not LGPL — this is a problem for some projects).

### Platform Codecs (Ranks 5-7)
- **Fraunhofer**: Embedded in commercial software. You don't choose it directly — it comes with the product. Found in older Windows Media Player versions (via DirectShow filter l3codecx.ax), various hardware decoders.
- **Apple CoreAudio**: All Apple devices. Handles MP3 natively. No user choice involved.
- **Windows Media Foundation**: All modern Windows. Native MP3 decode since Windows 7. MFT-based (Media Foundation Transform).

---

## Summary: Which Decoder to Use?

| Use Case | Recommended | Why |
|----------|-------------|-----|
| Library for any platform | **mpg123 (libmpg123)** | Fastest, most accurate, actively maintained, LGPL |
| Media application | **FFmpeg** | Already in your dependency tree. Solid. |
| Single-header embedding | **minimp3 (lieff)** | Zero deps, public domain, ISO compliant |
| Legacy codebase still using libmad | **Migrate to mpg123** | libmad is dead. mpg123 is better in every way. |
| macOS/iOS | **CoreAudio** | It's free and already there |
| Windows | **Media Foundation** | It's free and already there |

---

## Compliance and Accuracy

The ISO standard defines maximum allowed error for a compliant decoder:

| Metric | Requirement |
|--------|-------------|
| Maximum RMS error | 8.81 × 10⁻⁶ (relative to full scale) |
| Maximum signal difference | 6.10 × 10⁻⁵ |

Both mpg123 (float) and minimp3 pass these tests. libmad passes with its 24-bit fixed-point output. Integer-output decoders need careful rounding to comply.