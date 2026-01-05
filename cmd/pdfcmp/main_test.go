package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.pdf")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a test directory
	testDir := filepath.Join(tmpDir, "testdir")
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "ValidFile",
			path:    testFile,
			wantErr: false,
		},
		{
			name:        "FileNotFound",
			path:        filepath.Join(tmpDir, "nonexistent.pdf"),
			wantErr:     true,
			errContains: "file not found",
		},
		{
			name:        "IsDirectory",
			path:        testDir,
			wantErr:     true,
			errContains: "expected a file but got a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("validateFile() error = %v, should contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateFilePermission(t *testing.T) {
	// Skip on Windows where permission handling is different
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir := t.TempDir()

	// Create a file with no read permissions
	noReadFile := filepath.Join(tmpDir, "noread.pdf")
	if err := os.WriteFile(noReadFile, []byte("test"), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(noReadFile, 0644) // Restore for cleanup

	// Note: This test may not work if running as root
	err := validateFile(noReadFile)
	if err == nil {
		// Running as root or permission check doesn't work as expected
		t.Skip("Permission test skipped (possibly running as root)")
	}

	if !contains(err.Error(), "permission denied") && !contains(err.Error(), "cannot access") {
		t.Errorf("Expected permission error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
