package ignore_test

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
)

// This example packs a source tree into one prompt-ready blob, the job of an
// LLM context packer. go-ignore decides which paths are in: every .gitignore
// from the root down, an AI-specific .aiignore, and overrides that keep
// secrets out whatever a nested file says. The caller adds its own content
// policy: no dot-directories, no large or binary files. For a repository on
// disk, ignore.RepoEntries(root, opts, wo) replaces NewWithOptions and
// EntriesFS and also loads core.excludesFile and .git/info/exclude.
func ExampleMatcher_EntriesFS_contextPack() {
	fsys := fstest.MapFS{
		".gitignore":                {Data: []byte("*.log\n*.pem\n")},
		".aiignore":                 {Data: []byte("src/generated/\n")},
		"README.md":                 {Data: []byte("# demo\n")},
		"src/.cache/blob":           {Data: []byte("cached\n")},
		"src/app.log":               {Data: []byte("log line\n")},
		"src/big.sql":               {Data: []byte(strings.Repeat("-- filler\n", 7<<10))},
		"src/generated/api.go":      {Data: []byte("package generated\n")},
		"src/logo.png":              {Data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")},
		"src/main.go":               {Data: []byte("package main\n")},
		"src/testdata/.gitignore":   {Data: []byte("!*.pem\n")},
		"src/testdata/cert.pem":     {Data: []byte("-----BEGIN CERTIFICATE-----\n")},
		"src/testdata/fixture.json": {Data: []byte(`{"ok": true}`)},
	}
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		IgnoreFiles: []string{".aiignore"},
		Overrides:   []string{"*.pem"},
	})
	for e, err := range m.EntriesFS(fsys, ".", ignore.WalkOptions{Start: "src"}) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") {
				e.SkipDir()
			}
			continue
		}
		skipped, err := packFile(os.Stdout, fsys, e.Path, 64<<10)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		if skipped != "" {
			fmt.Printf("<!-- skipped %s: %s -->\n", e.Path, skipped)
		}
	}
	// Output:
	// <!-- skipped src/big.sql: over 64 KiB -->
	// <!-- skipped src/logo.png: binary -->
	// <file path="src/main.go">
	// package main
	// </file>
	// <file path="src/testdata/.gitignore">
	// !*.pem
	// </file>
	// <file path="src/testdata/fixture.json">
	// {"ok": true}
	// </file>
}

// packFile writes name as a <file> block, or returns why it was left out:
// larger than maxBytes (checked before reading), or binary (a NUL byte in
// the first 512 bytes, the heuristic most packers use).
func packFile(w io.Writer, fsys fs.FS, name string, maxBytes int64) (skipped string, err error) {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return "", err
	}
	if info.Size() > maxBytes {
		return fmt.Sprintf("over %d KiB", maxBytes>>10), nil
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(data[:min(len(data), 512)], 0) >= 0 {
		return "binary", nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	_, err = fmt.Fprintf(w, "<file path=\"%s\">\n%s</file>\n", html.EscapeString(name), data)
	return "", err
}
