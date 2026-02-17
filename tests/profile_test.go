package tests_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"
)

// TestProfileDecode runs saprobe-only decoding for clean pprof profiling.
// Use with: hack/bench.sh TestProfileDecode
//
//nolint:paralleltest // Profile must run sequentially for accurate sampling.
func TestProfileDecode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profile in short mode")
	}

	tmpDir := t.TempDir()

	var results []agar.BenchResult

	for _, bc := range mp3BenchConfigs {
		t.Logf("=== %s ===", bc.Name)

		srcPCM := agar.GenerateWhiteNoise(bc.SampleRate, 16, bc.Channels, benchDuration)
		srcPath := filepath.Join(tmpDir, fmt.Sprintf("src_%s.raw", bc.Name))

		if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}

		t.Logf("  PCM size: %.1f MB (%d bytes)", float64(len(srcPCM))/(1024*1024), len(srcPCM))

		// Encode with ffmpeg (untimed setup).
		encPath := filepath.Join(tmpDir, fmt.Sprintf("enc_%s.mp3", bc.Name))

		agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
			Src: srcPath, Dst: encPath,
			BitDepth: 16, SampleRate: bc.SampleRate, Channels: bc.Channels,
			CodecArgs: bc.FFmpegArgs,
		})

		encoded, err := os.ReadFile(encPath)
		if err != nil {
			t.Fatalf("read encoded: %v", err)
		}

		t.Logf("  MP3 size: %.1f MB (%d bytes)", float64(len(encoded))/(1024*1024), len(encoded))

		// Benchmark saprobe decode only.
		results = append(results, benchMP3DecodeSaprobe(t, bc.BenchFormat, encoded))
	}

	agar.PrintResults(t, benchOpts, results)
}

// TestProfileDecodeFile runs saprobe-only decoding of a real file for clean pprof profiling.
// Set BENCH_MP3_FILE to an MP3 file path.
// Use with: hack/bench.sh TestProfileDecodeFile /path/to/file.mp3
//
//nolint:paralleltest // Profile must run sequentially for accurate sampling.
func TestProfileDecodeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profile in short mode")
	}

	filePath := os.Getenv("BENCH_MP3_FILE")
	if filePath == "" {
		t.Skip("set BENCH_MP3_FILE to run this profile")
	}

	encoded, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	t.Logf("File: %s (%.1f MB)", filePath, float64(len(encoded))/(1024*1024))

	bf := agar.BenchFormat{Name: filepath.Base(filePath)}

	results := []agar.BenchResult{benchMP3DecodeSaprobe(t, bf, encoded)}

	agar.PrintResults(t, benchOpts, results)
}
