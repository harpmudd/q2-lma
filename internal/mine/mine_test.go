package mine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToggleKeepsCommentsAndOrder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "my-artists.txt")
	os.WriteFile(p, []byte("# mine\r\nGratefulDead\tGrateful Dead\r\n\r\nmoe   moe.\r\nnot an id!\r\n"), 0o644)
	l := Load(p)
	es := l.Entries()
	if len(es) != 3 || es[0] != (Entry{"GratefulDead", "Grateful Dead"}) || es[1] != (Entry{"moe", "moe."}) ||
		es[2].ID != "not" {
		t.Fatalf("%+v", es)
	}
	if added, err := l.Toggle("moe", "moe."); added || err != nil {
		t.Fatalf("remove: %v %v", added, err)
	}
	if added, err := l.Toggle("UmphreysMcGee", "Umphreys McGee"); !added || err != nil {
		t.Fatalf("add: %v %v", added, err)
	}
	b, _ := os.ReadFile(p)
	want := "# mine\nGratefulDead\tGrateful Dead\n\nnot an id!\nUmphreysMcGee\tUmphreys McGee\n"
	if string(b) != want {
		t.Errorf("file:\n%s", b)
	}
	if !Load(p).Has("UmphreysMcGee") || Load(p).Has("moe") {
		t.Error("reload")
	}
}

func TestBOM(t *testing.T) {
	p := filepath.Join(t.TempDir(), "my-artists.txt")
	os.WriteFile(p, []byte("\xef\xbb\xbfGratefulDead\tGrateful Dead\n"), 0o644) // Notepad's UTF-8
	if es := Load(p).Entries(); len(es) != 1 || es[0].ID != "GratefulDead" {
		t.Errorf("%+v", es)
	}
}

func TestFirstAddWritesHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "my-artists.txt")
	l := Load(p)
	if len(l.Entries()) != 0 {
		t.Fatal("not empty")
	}
	if _, err := l.Toggle("GratefulDead", "Grateful Dead"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "# My Artists") || !strings.HasSuffix(string(b), "GratefulDead\tGrateful Dead\n") {
		t.Errorf("%s", b)
	}
	if _, err := l.Toggle("../etc", "x"); err == nil {
		t.Error("bad id taken")
	}
}
