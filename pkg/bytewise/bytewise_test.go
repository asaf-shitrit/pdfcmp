package bytewise

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
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

// within fails the test, instead of hanging it, when fn does not return in time.
func within(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("call did not return within 5s")
	}
}

func TestExportedFuncsRejectNonRegular(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "reg")
	if err := os.WriteFile(reg, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("cannot create a FIFO: %v", err)
	}
	funcs := map[string]func(a, b string) error{
		"Compare":              func(a, b string) error { _, err := Compare(a, b); return err },
		"QuickCompare":         func(a, b string) error { _, err := QuickCompare(a, b); return err },
		"CompareWithEarlyExit": func(a, b string) error { _, err := CompareWithEarlyExit(a, b); return err },
		"HashFile":             func(a, _ string) error { _, err := HashFile(a); return err },
	}
	pairs := [][2]string{{fifo, reg}, {reg, fifo}, {fifo, fifo}, {"/dev/zero", "/dev/zero"}, {"/dev/zero", reg}, {dir, reg}}
	for name, fn := range funcs {
		for _, p := range pairs {
			if name == "HashFile" && p[0] == reg {
				continue // HashFile reads one path only
			}
			t.Run(name+"/"+filepath.Base(p[0])+"-"+filepath.Base(p[1]), func(t *testing.T) {
				var err error
				within(t, func() { err = fn(p[0], p[1]) })
				if err == nil {
					t.Error("want error for a non-regular file")
				}
			})
		}
	}
}

func TestExportedFuncsUnreadableIsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	dir := t.TempDir()
	noread := filepath.Join(dir, "noread")
	longer := filepath.Join(dir, "longer")
	for path, data := range map[string]string{noread: "abcd", longer: "abcdef"} {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(noread, 0); err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{noread, longer} {
		if _, err := QuickCompare(noread, other); err == nil {
			t.Errorf("QuickCompare(noread, %s): want error", filepath.Base(other))
		}
		if _, err := CompareWithEarlyExit(other, noread); err == nil {
			t.Errorf("CompareWithEarlyExit(%s, noread): want error", filepath.Base(other))
		}
	}
	if _, err := HashFile(noread); err == nil {
		t.Error("HashFile(noread): want error")
	}
}

func TestCompareWithEarlyExitSizesAndTail(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, n int, last byte) string {
		p := filepath.Join(dir, name)
		b := make([]byte, n)
		if n > 0 {
			b[n-1] = last
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"empty", write("e1", 0, 0), write("e2", 0, 0), true},
		{"exact chunk", write("c1", bufferSize, 1), write("c2", bufferSize, 1), true},
		{"chunk plus one", write("p1", bufferSize+1, 1), write("p2", bufferSize+1, 1), true},
		{"differ in last byte", write("d1", bufferSize+1, 1), write("d2", bufferSize+1, 2), false},
		{"size differs", write("s1", 10, 0), write("s2", 11, 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareWithEarlyExit(tt.a, tt.b)
			if err != nil || got != tt.want {
				t.Errorf("got (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}
