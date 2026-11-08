package bytewise

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCompare(t *testing.T) {
	tmpDir := t.TempDir()

	f1 := filepath.Join(tmpDir, "f1")
	f2 := filepath.Join(tmpDir, "f2")
	f3 := filepath.Join(tmpDir, "f3")

	for path, data := range map[string]string{f1: "hello world", f2: "hello world", f3: "hello world!"} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("Identical", func(t *testing.T) {
		res, err := Compare(f1, f2)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Identical {
			t.Error("Files should be identical")
		}
		if res.Hash1 != res.Hash2 {
			t.Error("Hashes should match")
		}
	})

	t.Run("DifferentContent", func(t *testing.T) {
		res, err := Compare(f1, f3)
		if err != nil {
			t.Fatal(err)
		}
		if res.Identical {
			t.Error("Files should be different")
		}
	})
}

func TestCompareWithEarlyExit(t *testing.T) {
	tmpDir := t.TempDir()

	f1 := filepath.Join(tmpDir, "f1")
	f2 := filepath.Join(tmpDir, "f2")
	f3 := filepath.Join(tmpDir, "f3")

	content := make([]byte, 1024*1024) // 1MB
	for i := range content {
		content[i] = byte(i % 256)
	}

	os.WriteFile(f1, content, 0644)
	os.WriteFile(f2, content, 0644)

	content[500] = 0 // Modification in the middle
	os.WriteFile(f3, content, 0644)

	t.Run("Identical", func(t *testing.T) {
		equal, err := CompareWithEarlyExit(f1, f2)
		if err != nil {
			t.Fatal(err)
		}
		if !equal {
			t.Error("Files should be equal")
		}
	})

	t.Run("Different", func(t *testing.T) {
		equal, err := CompareWithEarlyExit(f1, f3)
		if err != nil {
			t.Fatal(err)
		}
		if equal {
			t.Error("Files should be different")
		}
	})
}

func TestCompareRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "reg")
	if err := os.WriteFile(reg, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("cannot create a FIFO: %v", err)
	}
	for _, p := range [][2]string{{fifo, reg}, {reg, fifo}, {fifo, fifo}, {"/dev/zero", "/dev/zero"}, {dir, reg}} {
		if _, err := Compare(p[0], p[1]); err == nil {
			t.Errorf("Compare(%s, %s): want error for a non-regular file", p[0], p[1])
		}
	}
}

func TestCompareUnreadableIsErrorWhateverTheSize(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	dir := t.TempDir()
	noread := filepath.Join(dir, "noread")
	same := filepath.Join(dir, "same")
	longer := filepath.Join(dir, "longer")
	for path, data := range map[string]string{noread: "abcd", same: "abcd", longer: "abcdef"} {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(noread, 0); err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{same, longer} {
		if _, err := Compare(noread, other); err == nil {
			t.Errorf("Compare(noread, %s): want permission error", filepath.Base(other))
		}
		if _, err := Compare(other, noread); err == nil {
			t.Errorf("Compare(%s, noread): want permission error", filepath.Base(other))
		}
	}
}
