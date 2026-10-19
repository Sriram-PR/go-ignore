package ignore

import (
	"path/filepath"
	"testing"
)

// On disk, Source is the OS path of the deciding .gitignore, and Check stats
// the path to tell a directory from a file.
func TestTree_OS(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":     "build/\n",
		"src/.gitignore": "*.tmp\n",
		"build":          "DIR",
		"src/build":      "",
	})
	tr := New().Tree(root)
	res := mustCheck(t, tr, "src/x.tmp")
	if want := filepath.Join(root, "src", ".gitignore"); !res.Ignored || res.Source != want {
		t.Errorf("src/x.tmp: %+v, want ignored with Source %s", res, want)
	}
	if !mustCheck(t, tr, "build").Ignored {
		t.Error("build (a directory) not ignored by build/")
	}
	if mustCheck(t, tr, "src/build").Ignored {
		t.Error("src/build (a file) ignored by build/")
	}
}

// A path below a file asks for a .gitignore under that file, which is no
// .gitignore at all, not an error.
func TestTree_OSPathBelowFile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{".gitignore": "x\n", "file.txt": ""})
	res, err := New().Tree(root).Check("file.txt/x")
	if err != nil || !res.Ignored {
		t.Errorf("Check(file.txt/x) = %+v, %v; want ignored by x, no error", res, err)
	}
}
