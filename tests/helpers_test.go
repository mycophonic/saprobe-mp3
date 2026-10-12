package tests_test

import (
	"io"
	"os"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"

	mp3 "github.com/mycophonic/saprobe-mp3"
)

// decodeMp3 decodes an MP3 file using the saprobe decoder, failing the test
// on any error, the file's Close included.
func decodeMp3(t *testing.T, path string) ([]byte, mp3.PCMFormat) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}

	t.Cleanup(func() {
		if err = f.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	})

	dec, err := mp3.NewDecoder(f)
	if err != nil {
		t.Fatalf("new decoder for %s: %v", path, err)
	}

	pcm, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}

	return pcm, dec.Format()
}

// Benchmark infrastructure.

var benchOpts = agar.BenchOptions{}.WithDefaults() //nolint:gochecknoglobals // shared benchmark options, read-only

const benchDuration = 10 // seconds of audio
