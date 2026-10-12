package tests_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"

	mp3 "github.com/mycophonic/saprobe-mp3"
)

// mp3BenchConfig describes a synthetic MP3 benchmark format.
type mp3BenchConfig struct {
	agar.BenchFormat

	FFmpegArgs []string
}

//nolint:gochecknoglobals // the benchmark matrix, read-only test data
var mp3BenchConfigs = []mp3BenchConfig{
	{
		agar.BenchFormat{Name: "44.1kHz stereo CBR320", SampleRate: 44100, Channels: 2},
		[]string{"-c:a", "libmp3lame", "-b:a", "320k"},
	},
	{
		agar.BenchFormat{Name: "44.1kHz stereo CBR128", SampleRate: 44100, Channels: 2},
		[]string{"-c:a", "libmp3lame", "-b:a", "128k"},
	},
	{
		agar.BenchFormat{Name: "44.1kHz stereo VBR q2", SampleRate: 44100, Channels: 2},
		[]string{"-c:a", "libmp3lame", "-q:a", "2"},
	},
}

//nolint:paralleltest // Benchmark must run sequentially for accurate timing.
func TestMP3BenchmarkDecode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	lameBin, lameBinErr := agar.LookFor("lame")
	mpg123Bin, mpg123BinErr := agar.LookFor("mpg123")
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

		// Encode with ffmpeg.
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

		// Benchmark saprobe and ffmpeg decode.
		results = append(results,
			benchMP3DecodeSaprobe(t, bc.BenchFormat, encoded),
			benchMP3DecodeFFmpeg(t, bc, encPath),
		)

		// Benchmark lame decode.
		if lameBinErr == nil {
			results = append(results, benchMP3DecodeLame(t, bc.BenchFormat, lameBin, encPath))
		}

		// Benchmark mpg123 decode.
		if mpg123BinErr == nil {
			results = append(results, benchMP3DecodeMpg123(t, bc.BenchFormat, mpg123Bin, encPath))
		}

		// CoreAudio decode (CGO, in-process).
		results = append(results, agar.BenchDecodeCoreAudio(t, bc.BenchFormat, benchOpts, encoded))
	}

	agar.PrintResults(t, benchOpts, results)
}

//nolint:paralleltest // Benchmark must run sequentially for accurate timing.
func TestMP3BenchmarkDecodeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	filePath := os.Getenv("BENCH_MP3_FILE")
	if filePath == "" {
		t.Skip("set BENCH_MP3_FILE to run this benchmark")
	}

	encoded, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	t.Logf("File: %s (%.1f MB)", filePath, float64(len(encoded))/(1024*1024))

	tmpDir := t.TempDir()
	lameBin, lameBinErr := agar.LookFor("lame")
	mpg123Bin, mpg123BinErr := agar.LookFor("mpg123")

	bf := agar.BenchFormat{Name: filepath.Base(filePath)}

	var results []agar.BenchResult

	// saprobe decode.
	results = append(results, benchMP3DecodeSaprobe(t, bf, encoded))

	// ffmpeg decode.
	mp3Path := filepath.Join(tmpDir, "input.mp3")
	if writeErr := os.WriteFile(mp3Path, encoded, 0o600); writeErr != nil {
		t.Fatalf("write temp: %v", writeErr)
	}

	bc := mp3BenchConfig{BenchFormat: agar.BenchFormat{Name: bf.Name, SampleRate: 44100, Channels: 2}}
	results = append(results, benchMP3DecodeFFmpeg(t, bc, mp3Path))

	// lame decode.
	if lameBinErr == nil {
		results = append(results, benchMP3DecodeLame(t, bf, lameBin, mp3Path))
	}

	// mpg123 decode.
	if mpg123BinErr == nil {
		results = append(results, benchMP3DecodeMpg123(t, bf, mpg123Bin, mp3Path))
	}

	// CoreAudio decode (CGO, in-process).
	results = append(results, agar.BenchDecodeCoreAudio(t, bf, benchOpts, encoded))

	agar.PrintResults(t, benchOpts, results)
}

func benchMP3DecodeSaprobe(t *testing.T, bf agar.BenchFormat, encoded []byte) agar.BenchResult {
	t.Helper()

	durations := make([]time.Duration, benchOpts.Iterations)
	outBuf := make([]byte, 64*1024)

	for iter := range benchOpts.Iterations {
		start := time.Now()

		dec, err := mp3.NewDecoder(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("saprobe decode: %v", err)
		}

		for {
			_, readErr := dec.Read(outBuf)
			if readErr != nil {
				break
			}
		}

		durations[iter] = time.Since(start)
	}

	return agar.ComputeResult(bf, "saprobe", "decode", durations, len(encoded))
}

func benchMP3DecodeFFmpeg(t *testing.T, bc mp3BenchConfig, srcPath string) agar.BenchResult {
	t.Helper()

	durations := make([]time.Duration, benchOpts.Iterations)

	for iter := range benchOpts.Iterations {
		start := time.Now()

		agar.FFmpegDecode(t, agar.FFmpegDecodeOptions{
			Src: srcPath, BitDepth: 16, Channels: bc.Channels,
			Stdout: io.Discard,
		})

		durations[iter] = time.Since(start)
	}

	return agar.ComputeResult(bc.BenchFormat, "ffmpeg", "decode", durations, agar.FileSize(t, srcPath))
}

func benchMP3DecodeLame(t *testing.T, bf agar.BenchFormat, lameBin, srcPath string) agar.BenchResult {
	t.Helper()

	durations := make([]time.Duration, benchOpts.Iterations)

	for iter := range benchOpts.Iterations {
		start := time.Now()

		cmd := exec.Command(lameBin, "--decode", "-t", "--quiet", srcPath, "-")

		var stdout, stderr bytes.Buffer

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf("lame decode: %v\n%s", err, stderr.String())
		}

		durations[iter] = time.Since(start)
	}

	return agar.ComputeResult(bf, "lame", "decode", durations, agar.FileSize(t, srcPath))
}

func benchMP3DecodeMpg123(t *testing.T, bf agar.BenchFormat, mpg123Bin, srcPath string) agar.BenchResult {
	t.Helper()

	durations := make([]time.Duration, benchOpts.Iterations)

	for iter := range benchOpts.Iterations {
		start := time.Now()

		cmd := exec.Command(mpg123Bin, "--quiet", "-s", srcPath)

		var stdout, stderr bytes.Buffer

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf("mpg123 decode: %v\n%s", err, stderr.String())
		}

		durations[iter] = time.Since(start)
	}

	return agar.ComputeResult(bf, "mpg123", "decode", durations, agar.FileSize(t, srcPath))
}
