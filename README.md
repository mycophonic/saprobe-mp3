# Saprobe MP3

Pure Go MP3 streaming decoder with LAME gapless playback support.

No encoder.

An example cli decoder is provided.

For a full-blown cli, see [Saprobe](https://github.com/mycophonic/saprobe).

## Support

- **MPEG versions:** 1, 2, 2.5
- **Layers:** III
- **Bitrates:** all standard CBR and VBR
- **Sample rates:** all standard (8000-48000 Hz)
- **Channels:** mono, stereo, joint stereo, dual channel
- **Gapless:** LAME encoder delay/padding tags
- **Output:** interleaved little-endian signed 16-bit PCM (always stereo)

## API

```go
func NewDecoder(rs io.ReadSeeker) (*Decoder, error)
func (d *Decoder) Read(p []byte) (int, error)
func (d *Decoder) Format() PCMFormat

func Decode(reader io.ReadSeeker) ([]byte, PCMFormat, error)
```

## Dependencies

None (pure Go).

## Detailed documentation

* [MP3 format notes](./docs/MP3.md)
* [decoders landscape](./docs/research/DECODERS.md)
* [encoders landscape](./docs/research/ENCODERS.md)
* [tests and benchmarks](./docs/QA.md)
