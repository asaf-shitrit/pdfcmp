package bytewise

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompare(t *testing.T) {
	tmpDir := t.TempDir()
	
	f1 := filepath.Join(tmpDir, "f1")
	f2 := filepath.Join(tmpDir, "f2")
	f3 := filepath.Join(tmpDir, "f3")
	
	os.WriteFile(f1, []byte("hello world"), 0644)
	os.WriteFile(f2, []byte("hello world"), 0644)
	os.WriteFile(f3, []byte("hello world!"), 0644)
	
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
