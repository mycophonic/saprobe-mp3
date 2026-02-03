package tests_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"slices"
	"testing"
	"time"

	mp3 "github.com/mycophonic/saprobe-mp3"
)

// decodeMp3 decodes an MP3 file using the saprobe decoder.
func decodeMp3(path string) ([]byte, mp3.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, mp3.PCMFormat{}, err
	}
	defer f.Close()

	return mp3.Decode(f)
}

// generateWhiteNoise creates random 16-bit stereo PCM data.
func generateWhiteNoise(sampleRate, bitDepth, channels, durationSec int) []byte {
	numSamples := sampleRate * durationSec * channels
	bytesPerSample := pcmBytesPerSample(bitDepth)

	buf := make([]byte, numSamples*bytesPerSample)

	// Use a simple PRNG for reproducibility.
	seed := uint64(0x12345678)

	for i := range numSamples {
		// xorshift64
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17

		offset := i * bytesPerSample

		// MP3 only uses 16-bit.
		val := int16((seed % 60000) - 30000)
		binary.LittleEndian.PutUint16(buf[offset:], uint16(val))
	}

	return buf
}

// compareLossySamples allows small differences between decoders for lossy codecs.
// Different MP3 decoders use different floating-point implementations,
// resulting in ±1-2 LSB differences per sample.
// Length differences up to 1 MP3 frame (1152 stereo samples = 4608 bytes) are tolerated.
func compareLossySamples(t *testing.T, ffmpegPCM, saprobePCM []byte, bitDepth, channels int) {
	t.Helper()

	// Only 16-bit lossy codecs are currently supported.
	if bitDepth != 16 {
		t.Errorf("lossy comparison only supports 16-bit, got %d-bit", bitDepth)

		return
	}

	// Allow length differences up to 1 frame (1152 samples * channels * 2 bytes).
	const samplesPerFrame = 1152

	maxLengthDiffBytes := samplesPerFrame * channels * 2

	lengthDiff := len(saprobePCM) - len(ffmpegPCM)
	if lengthDiff < 0 {
		lengthDiff = -lengthDiff
	}

	if lengthDiff > maxLengthDiffBytes {
		t.Errorf("length mismatch: ffmpeg=%d, saprobe=%d (diff=%d exceeds tolerance %d)",
			len(ffmpegPCM), len(saprobePCM), lengthDiff, maxLengthDiffBytes)

		return
	}

	if lengthDiff > 0 {
		t.Logf("length diff: ffmpeg=%d, saprobe=%d (±%d bytes, within tolerance)",
			len(ffmpegPCM), len(saprobePCM), lengthDiff)
	}

	numSamples := min(len(ffmpegPCM), len(saprobePCM)) / 2

	const maxDiffPerSample = 2 // Allow ±2 difference per 16-bit sample

	largeDiffs := 0
	maxDiff := int16(0)

	for i := range numSamples {
		ffSample := int16(binary.LittleEndian.Uint16(ffmpegPCM[i*2:]))
		spSample := int16(binary.LittleEndian.Uint16(saprobePCM[i*2:]))

		diff := ffSample - spSample
		if diff < 0 {
			diff = -diff
		}

		if diff > maxDiffPerSample {
			largeDiffs++
		}

		if diff > maxDiff {
			maxDiff = diff
		}
	}

	// Allow up to 1% of samples to have larger differences (codec edge cases).
	maxLargeDiffs := numSamples / 100
	if largeDiffs > maxLargeDiffs {
		t.Errorf("lossy PCM mismatch: %d samples (%.2f%%) differ by more than ±%d, max diff=%d",
			largeDiffs, float64(largeDiffs)/float64(numSamples)*100, maxDiffPerSample, maxDiff)
		showDiffs(t, "saprobe vs ffmpeg", ffmpegPCM, saprobePCM, bitDepth, channels, 5)
	}
}

// monoToStereo duplicates mono PCM to stereo by copying each sample to both channels.
func monoToStereo(mono []byte, bitDepth int) []byte {
	bps := pcmBytesPerSample(bitDepth)
	numSamples := len(mono) / bps
	stereo := make([]byte, numSamples*bps*2)

	for i := range numSamples {
		sample := mono[i*bps : (i+1)*bps]
		copy(stereo[i*bps*2:], sample)
		copy(stereo[i*bps*2+bps:], sample)
	}

	return stereo
}

// pcmBytesPerSample returns the number of bytes per sample for a given bit depth.
func pcmBytesPerSample(bitDepth int) int {
	switch bitDepth {
	case 4, 8:
		return 1
	case 12, 16:
		return 2
	case 20, 24:
		return 3
	case 32:
		return 4
	default:
		return bitDepth / 8
	}
}

// showDiffs prints the first N differing samples for debugging.
func showDiffs(t *testing.T, label string, expected, actual []byte, bitDepth, channels, maxDiffs int) {
	t.Helper()

	bytesPerSample := pcmBytesPerSample(bitDepth)
	frameSize := bytesPerSample * channels
	shown := 0

	for i := 0; i < min(len(expected), len(actual))-frameSize && shown < maxDiffs; i += frameSize {
		expectedFrame := expected[i : i+frameSize]
		actualFrame := actual[i : i+frameSize]

		if !bytes.Equal(expectedFrame, actualFrame) {
			sampleIdx := i / frameSize
			t.Logf("%s: sample %d: expected=%v, actual=%v", label, sampleIdx, expectedFrame, actualFrame)

			shown++
		}
	}
}

// Benchmark infrastructure.

const (
	benchIterations = 10
	benchDuration   = 10 // seconds of audio
)

type benchResult struct {
	Format  string
	Tool    string
	Op      string
	Median  time.Duration
	Mean    time.Duration
	Min     time.Duration
	Max     time.Duration
	Stddev  time.Duration
	PCMSize int
}

func computeResult(format, tool, op string, durations []time.Duration, pcmSize int) benchResult {
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	slices.Sort(sorted)

	var sum float64
	for _, d := range durations {
		sum += float64(d)
	}

	mean := sum / float64(len(durations))

	var variance float64

	for _, d := range durations {
		diff := float64(d) - mean
		variance += diff * diff
	}

	variance /= float64(len(durations))

	return benchResult{
		Format:  format,
		Tool:    tool,
		Op:      op,
		Median:  sorted[len(sorted)/2],
		Mean:    time.Duration(mean),
		Min:     sorted[0],
		Max:     sorted[len(sorted)-1],
		Stddev:  time.Duration(math.Sqrt(variance)),
		PCMSize: pcmSize,
	}
}

func printResults(t *testing.T, results []benchResult) {
	t.Helper()

	sep := "──────────────────────────────────────────────────────────────────"

	t.Log("")
	t.Log("┌" + sep + "┐")
	t.Logf("│ MP3 Benchmark Results (%d iterations per test)%s│",
		benchIterations, "                   ")
	t.Log("├" + sep + "┤")
	t.Logf("│ %-24s %-7s %-6s %8s %8s %8s %8s│",
		"Format", "Tool", "Op", "Median", "Mean", "Min", "Max")
	t.Log("├" + sep + "┤")

	currentFormat := ""

	for _, r := range results {
		if r.Format != currentFormat {
			if currentFormat != "" {
				t.Log("├" + sep + "┤")
			}

			currentFormat = r.Format
		}

		t.Logf("│ %-24s %-7s %-6s %8s %8s %8s %8s│",
			r.Format, r.Tool, r.Op,
			r.Median.Round(time.Millisecond),
			r.Mean.Round(time.Millisecond),
			r.Min.Round(time.Millisecond),
			r.Max.Round(time.Millisecond),
		)
	}

	t.Log("└" + sep + "┘")
}

func fileSize(t *testing.T, path string) int {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	return int(info.Size())
}
