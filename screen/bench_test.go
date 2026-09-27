package screen

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkFeed interprets 16 MB of real output — git log, ls and cat,
// recorded in tools/vtdiff — on a 120 × 40 screen, the measure go-ui.md
// used when choosing the emulator (libvterm: 20 MB/s; xterm-go as found:
// 86 MB/s).
func BenchmarkFeed(b *testing.B) {
	var chunk []byte
	for _, name := range []string{"gitlog", "ls", "cat"} {
		d, err := os.ReadFile(filepath.Join("testdata", "vtdiff", "streams", name+".bin"))
		if err != nil {
			b.Fatal(err)
		}
		chunk = append(chunk, d...)
	}
	var data []byte
	for len(data) < 16<<20 {
		data = append(data, chunk...)
	}
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		s := New(120, 40)
		for off := 0; off < len(data); off += 64 << 10 {
			s.Feed(data[off:min(off+64<<10, len(data))])
		}
	}
}
