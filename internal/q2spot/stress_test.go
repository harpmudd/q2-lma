package q2spot

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestStress asks many times in a row, each reply after a short wait, as the app's are: every reply
// must arrive whole. Run it with Q2SPOT_STRESS=1.
func TestStress(t *testing.T) {
	if os.Getenv("Q2SPOT_STRESS") == "" {
		t.Skip("set Q2SPOT_STRESS=1")
	}
	sock := filepath.Join(t.TempDir(), "s.sock")
	go Serve(sock, func(r Request) Reply {
		time.Sleep(5 * time.Millisecond)
		return Reply{Rows: []Row{{Kind: KindInfo, Title: "x"}}}
	})
	time.Sleep(100 * time.Millisecond)
	bad := 0
	for i := 0; i < 400; i++ {
		b, err := AskRaw(sock, strconv.Itoa(i), "menu")
		if err != nil || len(b) == 0 {
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%d of 400 replies empty", bad)
	}
}
