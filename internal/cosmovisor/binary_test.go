package cosmovisor

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "testfile.bin")
	content := []byte("hello world test content for sha256")

	if err := os.WriteFile(testFile, content, 0o600); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(h[:])

	t.Run("matching hash", func(t *testing.T) {
		err := VerifySHA256(testFile, expectedHash)
		if err != nil {
			t.Errorf("expected no error for matching hash, got: %v", err)
		}
	})

	t.Run("mismatched hash", func(t *testing.T) {
		err := VerifySHA256(testFile, "0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Error("expected error for mismatched hash")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		err := VerifySHA256(filepath.Join(dir, "nonexistent"), expectedHash)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestFindBinaryAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "inferenced-amd64.zip", DownloadURL: "https://example.com/inferenced.zip", SHA256: "abc123", Size: 98000000},
		{Name: "decentralized-api-amd64.zip", DownloadURL: "https://example.com/api.zip", SHA256: "def456", Size: 191000000},
		{Name: "checksums.txt", DownloadURL: "https://example.com/checksums.txt", Size: 256},
	}

	t.Run("find inferenced", func(t *testing.T) {
		a := FindBinaryAsset(assets, "inferenced-amd64.zip")
		if a == nil {
			t.Fatal("expected to find inferenced asset")
		}
		if a.SHA256 != "abc123" {
			t.Errorf("SHA256 = %q, want %q", a.SHA256, "abc123")
		}
	})

	t.Run("find api", func(t *testing.T) {
		a := FindBinaryAsset(assets, "decentralized-api-amd64.zip")
		if a == nil {
			t.Fatal("expected to find api asset")
		}
		if a.Size != 191000000 {
			t.Errorf("Size = %d, want %d", a.Size, 191000000)
		}
	})

	t.Run("not found", func(t *testing.T) {
		a := FindBinaryAsset(assets, "nonexistent.zip")
		if a != nil {
			t.Error("expected nil for nonexistent asset")
		}
	})

	t.Run("empty list", func(t *testing.T) {
		a := FindBinaryAsset(nil, "inferenced-amd64.zip")
		if a != nil {
			t.Error("expected nil for empty asset list")
		}
	})
}

func TestExtractZipBinary(t *testing.T) {
	dir := t.TempDir()

	zipPath := filepath.Join(dir, "test.zip")
	destPath := filepath.Join(dir, "extracted-binary")
	binaryContent := []byte("#!/bin/sh\necho 'hello'\n")

	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)

	fw, err := zw.Create("inferenced")
	if err != nil {
		t.Fatal(err)
	}
	if _, writeErr := fw.Write(binaryContent); writeErr != nil {
		t.Fatal(writeErr)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	t.Run("extract existing binary", func(t *testing.T) {
		err := ExtractZipBinary(zipPath, destPath, "inferenced", false)
		if err != nil {
			t.Fatalf("ExtractZipBinary() error: %v", err)
		}

		data, readErr := os.ReadFile(destPath)
		if readErr != nil {
			t.Fatalf("read extracted file: %v", readErr)
		}
		if string(data) != string(binaryContent) {
			t.Errorf("content mismatch: got %q, want %q", string(data), string(binaryContent))
		}
	})

	t.Run("binary not in zip", func(t *testing.T) {
		err := ExtractZipBinary(zipPath, filepath.Join(dir, "other"), "nonexistent", false)
		if err == nil {
			t.Error("expected error for missing binary in zip")
		}
	})

	t.Run("invalid zip file", func(t *testing.T) {
		invalidZip := filepath.Join(dir, "invalid.zip")
		if writeErr := os.WriteFile(invalidZip, []byte("not a zip"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		err := ExtractZipBinary(invalidZip, filepath.Join(dir, "out"), "inferenced", false)
		if err == nil {
			t.Error("expected error for invalid zip")
		}
	})
}
