package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyEnvFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env")
	body := "# comment\n\nDRIVER_TEST_A=hello world\nDRIVER_TEST_B = ~/x\nexport DRIVER_TEST_C=nope\nDRIVER_TEST_D\nDRIVER_TEST_A=second\nPATH=~/bin:/opt/x\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DRIVER_TEST_A", "")
	os.Unsetenv("DRIVER_TEST_A")
	t.Setenv("DRIVER_TEST_B", "preset")
	t.Setenv("PATH", "/usr/bin:/bin")
	applied := ApplyEnvFile(p)
	if got := os.Getenv("DRIVER_TEST_A"); got != "hello world" {
		t.Errorf("A = %q", got)
	}
	if got := os.Getenv("DRIVER_TEST_B"); got != "preset" {
		t.Errorf("B = %q, preset must win", got)
	}
	if _, ok := os.LookupEnv("export DRIVER_TEST_C"); ok {
		t.Errorf("export line must be skipped")
	}
	want := UserHome() + "/bin:/opt/x:/usr/bin:/bin"
	if got := os.Getenv("PATH"); got != want {
		t.Errorf("PATH = %q, want %q", got, want)
	}
	if strings.Join(applied, ",") != "DRIVER_TEST_A,PATH" {
		t.Errorf("applied = %v", applied)
	}
	if ApplyEnvFile(filepath.Join(dir, "missing")) != nil {
		t.Errorf("missing file must apply nothing")
	}
}

func TestOneLine(t *testing.T) {
	if got := OneLine("  a\n\tb   c\x00d ", 0); got != "a b c d" {
		t.Errorf("got %q", got)
	}
	if got := OneLine("abcdef", 4); got != "abc…" {
		t.Errorf("got %q", got)
	}
}

func TestExpandTilde(t *testing.T) {
	if ExpandTilde("~") != UserHome() || ExpandTilde("~/x") != filepath.Join(UserHome(), "x") || ExpandTilde("/a~") != "/a~" {
		t.Error("tilde expansion")
	}
}
