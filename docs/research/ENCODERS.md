# MP3 Encoders: Prevalence and Technical Specifications

## Overview

MP3 (MPEG-1/2/2.5 Audio Layer III) is a **lossy** codec. It does not have a user-selectable bit depth — the internal representation is always floating-point or fixed-point depending on the encoder, and the output is a perceptually coded bitstream. The relevant parameters are **bitrate**, **sample rate**, and **channel mode**.

MP3 supports **1–2 channels** (mono or stereo), sample rates from **8 kHz to 48 kHz** (across MPEG-1/2/2.5), and bitrates from **8 to 320 kbit/s**.

All MP3 patents expired by April 2017.

---

## Encoder Comparison Table

| Rank | Encoder | Bitrate Modes | Sample Rates | Channels | Platform | Notes |
|------|---------|---------------|--------------|----------|----------|-------|
| 1 | **LAME** | CBR, VBR, ABR | All MPEG-1/2/2.5 (8–48 kHz) | Mono, Stereo, Joint Stereo, Dual Mono | Cross-platform | The undisputed king. Best quality at mid/high bitrates. Best VBR implementation. LGPL. |
| 2 | **Fraunhofer IIS (FhG)** | CBR, VBR, ABR | All MPEG-1/2/2.5 | Mono, Stereo, Joint Stereo | Proprietary (embedded in apps) | Original MP3 creators. Better than LAME at low CBR bitrates (≤64 kbit/s speech). Not available standalone. |
| 3 | **Shine (libshine)** | **CBR only** | All MPEG-1/2/2.5 | Mono, Stereo | Cross-platform | Fixed-point encoder. Designed for FPU-less platforms (ARM, embedded). Significantly worse quality than LAME. Written by Gabriel Bouvigne (LAME contributor). |
| 4 | **FFmpeg (via libmp3lame)** | CBR, VBR, ABR | All MPEG-1/2/2.5 | Mono, Stereo, Joint Stereo | Cross-platform | **Not an independent encoder.** FFmpeg wraps LAME via `--enable-libmp3lame`. FFmpeg's own native MP3 encoder was removed years ago for being garbage. Also supports libshine as a fallback. |

---

## Historical / Obsolete Encoders

| Encoder | Era | Notes |
|---------|-----|-------|
| **Fraunhofer l3enc** | 1994–1997 | First public MP3 encoder ever. Shareware. 112 kbit/s limit without license. Obsolete. |
| **Fraunhofer MP3enc** | 1997–1998 | Successor to l3enc. Called the "gold standard" at the time. |
| **Fraunhofer Fastenc** | 1998+ | Successor to MP3enc. Optimized by Intel/AMD. ~5x faster than LAME. Discontinued. |
| **BladeEnc** | 1998–2002 | Open source. Quality identical to ISO dist10 reference (i.e., bad). |
| **Xing** | Late 1990s | Very fast, very poor quality. Used in early ripping software. |
| **GOGO-no-coda** | 2000s | Fork of LAME with assembly optimizations. Faster but no quality advantage. |
| **ISO dist10 reference** | 1992 | Reference implementation from ISO standard. Terrible quality. Educational only. |

---

## Detailed Notes

### LAME (Rank 1)
- **Usage**: De facto standard for all MP3 encoding worldwide. Used by Audacity (bundled since patents expired), most ripping software, most media pipelines.
- **Quality**: Considered the best MP3 encoder at 128+ kbit/s. VBR mode (-V 0 through -V 9) is its strongest feature — consistently transparent at -V 0 (~245 kbit/s average).
- **History**: Started 1998 as patches against ISO dist10 source. All ISO code removed by May 2000. Key contributors: Mark Taylor (gpsycho psychoacoustic model), Takehiro Tominaga, Gabriel Bouvigne, Robert Hegemann, Naoki Shibata.
- **Recommended settings**:
    - Transparent quality: `-V 0` (VBR, ~245 kbit/s)
    - Good quality: `-V 2` (VBR, ~190 kbit/s, default)
    - Maximum CBR: `-b 320 --cbr`
- **Weakness**: Mediocre at low CBR bitrates (≤64 kbit/s). Fraunhofer wins there.
- **Internal resampler**: Poor quality. Use SoX or SSRC for resampling before encoding.
- **Current version**: 3.100 (October 2017). Essentially mature — no significant changes expected.

### Fraunhofer IIS (Rank 2)
- **Usage**: Embedded in iTunes, Windows Media Player (older versions), Cubase, Logic Pro, Steinberg products, Adobe Audition, and other commercial software.
- **Not standalone**: Cannot be downloaded or purchased separately. Always licensed and embedded by third-party vendors.
- **Quality**: Better than LAME at low CBR bitrates (speech, podcasting at 64 kbit/s mono). At 128+ kbit/s, LAME matches or beats it, especially in VBR mode.
- **History**: Fraunhofer IIS co-invented the MP3 format itself. Their encoder lineage: l3enc → MP3enc → Fastenc → embedded SDK.

### Shine (Rank 3)
- **Usage**: Niche — embedded systems without FPU, Liquidsoap streaming software, educational.
- **Quality**: Significantly worse than LAME at all bitrates. Failed listening tests badly (Wikipedia: "all tested encoders significantly outperformed Shine").
- **Maintained by**: Savonet/Liquidsoap project (GitHub: savonet/shine). Original by Gabriel Bouvigne (early 2000s), dead for ~5 years, revived.
- **Features**: CBR only. No VBR. No psychoacoustic model tuning.
- **Use case**: When you absolutely cannot use floating-point math and need MP3. Otherwise, never use this.

### FFmpeg (Rank 4)
- **Reality check**: FFmpeg does not have its own MP3 encoder anymore. `libmp3lame` is required for MP3 encoding. The old native encoder (`ffmp3`) was removed for abysmal quality. When you run `ffmpeg -c:a mp3`, it silently uses libmp3lame if available.
- As a fallback, FFmpeg can also use libshine (`--enable-libshine`), but this is only useful for embedded/low-power scenarios.

---

## Summary: Which Encoder to Use?

| Use Case | Recommended | Why |
|----------|-------------|-----|
| Music, any bitrate ≥128 kbit/s | **LAME -V 0** or **-V 2** | Best quality, best VBR, transparent results |
| Music, maximum compatibility CBR | **LAME -b 320 --cbr** | Overkill but guaranteed transparency |
| Speech/podcast, low CBR (64 kbit/s mono) | **Fraunhofer** (via iTunes) | Slightly cleaner at low CBR than LAME |
| Embedded, no FPU | **Shine** | Only option. Quality is bad but it runs on anything |
| Scripted pipelines | **FFmpeg** (which uses LAME) | Convenience, not a distinct encoder |
| 2025+, new projects | **Don't use MP3.** Use Opus. | MP3 is 30+ years old. Opus beats it in every metric. |

---

## MP3 Format Specification Reference

| Parameter | MPEG-1 | MPEG-2 (LSF) | MPEG-2.5 |
|-----------|--------|--------------|----------|
| Sample rates | 32, 44.1, 48 kHz | 16, 22.05, 24 kHz | 8, 11.025, 12 kHz |
| Bitrates (Layer III) | 32–320 kbit/s (14 options) | 8–160 kbit/s (14 options) | 8–64 kbit/s (8 options) |
| Channels | 1–2 (Mono, Stereo, Joint Stereo, Dual Channel) | Same | Same |
| Frame size | 1152 samples (MPEG-1), 576 samples (MPEG-2/2.5) | — | — |
| Bit depth | N/A (lossy perceptual codec; input is typically 16-bit PCM) | — | — |
| Bit reservoir | Yes (frames can borrow bits from adjacent frames) | — | — |
| Standards | ISO/IEC 11172-3 (1993) | ISO/IEC 13818-3 (1995) | Non-standard (Fraunhofer extension) |

### Stereo Modes

| Mode | Description |
|------|-------------|
| Stereo | Independent L/R encoding |
| Joint Stereo | Mid/Side + Intensity stereo (adaptive per frame). Default and recommended. |
| Dual Channel | Two independent mono streams (e.g., two languages) |
| Mono | Single channel |