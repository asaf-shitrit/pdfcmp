package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestExitCodes runs the command line end to end and checks its exit status:
// 0 for files that match, 1 for files that differ, 2 when they cannot be
// compared or the command line is wrong.
func TestExitCodes(t *testing.T) {
	withFlags(t, nil)
	dir := t.TempDir()
	a := fixture(t, dir, "small_a.pdf")
	b := fixture(t, dir, "small_b.pdf")
	diff := fixture(t, dir, "small_diff_b.pdf")
	junk := filepath.Join(dir, "junk.pdf")
	if err := os.WriteFile(junk, []byte("not a pdf"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"compare same", []string{"compare", "-q", a, b}, exitSame},
		{"compare different", []string{"compare", "-q", a, diff}, exitDifferent},
		{"compare missing file", []string{"compare", "-q", a, filepath.Join(dir, "missing.pdf")}, exitError},
		{"compare not a pdf", []string{"compare", "-q", a, junk}, exitError},
		{"compare one argument", []string{"compare", a}, exitError},
		{"compare bad format", []string{"compare", "-q", "-f", "xml", a, b}, exitError},
		{"compare bad mode", []string{"compare", "-q", "-m", "nope", a, b}, exitError},
		{"compare timed out", []string{"compare", "-q", "--timeout", "1ns", a, diff}, exitError},
		{"compare negative timeout", []string{"compare", "-q", "--timeout", "-1s", a, diff}, exitError},
		{"quick identical", []string{"quick", a, a}, exitSame},
		{"quick different", []string{"quick", a, diff}, exitDifferent},
		{"quick missing file", []string{"quick", a, filepath.Join(dir, "missing.pdf")}, exitError},
		{"version", []string{"version"}, exitSame},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(tt.args); got != tt.want {
				t.Errorf("pdfcmp %v: exit %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })
	version = "v1.2.3"
	if got := versionString(); got != "v1.2.3" {
		t.Errorf("versionString() = %q, want the build-time version", got)
	}
	version = ""
	if got := versionString(); got == "" {
		t.Error("versionString() is empty without a build-time version")
	}
}

// TestExitCodesUnusableInput checks that a file which cannot be compared exits
// 2 whatever its size: a FIFO has no usable size, and an unreadable file used
// to exit 1 when its size differed from the other file's.
func TestExitCodesUnusableInput(t *testing.T) {
	withFlags(t, nil)
	dir := t.TempDir()
	a := fixture(t, dir, "small_a.pdf")
	diff := fixture(t, dir, "small_diff_b.pdf")

	fifo := filepath.Join(dir, "pipe.pdf")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("cannot create a FIFO: %v", err)
	}
	for _, args := range [][]string{{fifo, a}, {a, fifo}, {fifo, fifo}, {"/dev/zero", "/dev/zero"}} {
		for _, full := range [][]string{
			append([]string{"quick"}, args...),
			append([]string{"compare", "-q"}, args...),
		} {
			if got := run(full); got != exitError {
				t.Errorf("pdfcmp %v: exit %d, want %d", full, got, exitError)
			}
		}
	}

	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	noread := filepath.Join(dir, "noread.pdf")
	data, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noread, data, 0o000); err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{a, diff} { // equal size and different size
		for _, full := range [][]string{
			{"quick", noread, other},
			{"compare", "-q", noread, other},
		} {
			if got := run(full); got != exitError {
				t.Errorf("pdfcmp %v: exit %d, want %d", full, got, exitError)
			}
		}
	}
}
