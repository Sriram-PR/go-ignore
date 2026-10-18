package ignore

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestReadPatternFile_Limit(t *testing.T) {
	fsys := fstest.MapFS{
		"eight": {Data: []byte("12345678")},
		"nine":  {Data: []byte("123456789")},
	}
	src := fsSource{fsys}
	if got, err := readPatternFile(src, "eight", false, 8); err != nil || string(got) != "12345678" {
		t.Errorf("eight: got %q, %v; want the content", got, err)
	}
	if _, err := readPatternFile(src, "nine", false, 8); !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("nine: err = %v, want ErrPatternFileSkipped", err)
	}
	if _, err := readPatternFile(src, "missing", false, 8); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: err = %v, want fs.ErrNotExist", err)
	}
}

func TestReadPatternFile_Follow(t *testing.T) {
	fsys := fstest.MapFS{
		"target": {Data: []byte("x\n")},
		"link":   {Mode: fs.ModeSymlink, Data: []byte("target")},
	}
	src := fsSource{fsys}
	if _, err := readPatternFile(src, "link", false, 8); !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("follow=false: err = %v, want ErrPatternFileSkipped", err)
	}
	if got, err := readPatternFile(src, "link", true, 8); err != nil || string(got) != "x\n" {
		t.Errorf("follow=true: got %q, %v; want the target's content", got, err)
	}
}

func TestReadLimited(t *testing.T) {
	if got, err := readLimited(strings.NewReader("12345678"), 8); err != nil || string(got) != "12345678" {
		t.Errorf("8 bytes: got %q, %v", got, err)
	}
	if _, err := readLimited(strings.NewReader("123456789"), 8); !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("9 bytes: err = %v, want ErrPatternFileSkipped", err)
	}
}
