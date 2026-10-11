package tests_test

import (
	"io"
	"os"

	"github.com/mycophonic/agar/pkg/agar"

	mp3 "github.com/mycophonic/saprobe-mp3"
)

// decodeMp3 decodes an MP3 file using the saprobe decoder.
func decodeMp3(path string) ([]byte, mp3.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, mp3.PCMFormat{}, err
	}
	defer f.Close()

	dec, decErr := mp3.NewDecoder(f)
	if decErr != nil {
		return nil, mp3.PCMFormat{}, decErr
	}

	pcm, readErr := io.ReadAll(dec)
	if readErr != nil {
		return nil, mp3.PCMFormat{}, readErr
	}

	return pcm, dec.Format(), nil
}

// Benchmark infrastructure.

var benchOpts = agar.BenchOptions{}.WithDefaults() //nolint:gochecknoglobals // shared benchmark options, read-only

const benchDuration = 10 // seconds of audio
