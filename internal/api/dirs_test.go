package api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompleteDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"src", "scratch", ".ssh", "notes", "src/driver", "src/kraken"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "sfile"), nil, 0o644)
	os.Symlink(filepath.Join(root, "src"), filepath.Join(root, "slink"))

	got := CompleteDirs("", root)
	want := []string{filepath.Join(root, "notes"), filepath.Join(root, "scratch"), filepath.Join(root, "slink"), filepath.Join(root, "src")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty q: %v", got)
	}
	got = CompleteDirs(root+"/s", root)
	want = []string{filepath.Join(root, "scratch"), filepath.Join(root, "slink"), filepath.Join(root, "src")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("prefix s: %v", got)
	}
	got = CompleteDirs(root+"/.s", root)
	if !reflect.DeepEqual(got, []string{filepath.Join(root, ".ssh")}) {
		t.Errorf("hidden: %v", got)
	}
	got = CompleteDirs(root+"/src/", root)
	if !reflect.DeepEqual(got, []string{filepath.Join(root, "src", "driver"), filepath.Join(root, "src", "kraken")}) {
		t.Errorf("trailing slash: %v", got)
	}
	got = CompleteDirs("src/k", root)
	if !reflect.DeepEqual(got, []string{filepath.Join(root, "src", "kraken")}) {
		t.Errorf("relative: %v", got)
	}
	if got := CompleteDirs(root+"/nope/x", root); len(got) != 0 {
		t.Errorf("missing parent: %v", got)
	}
	t.Setenv("HOME", root)
	got = CompleteDirs("~/sc", "/")
	if !reflect.DeepEqual(got, []string{filepath.Join(root, "scratch")}) {
		t.Errorf("tilde: %v", got)
	}
}
