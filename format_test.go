package image_qcow2

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatName_QCOW2(t *testing.T) {
	if got := (Format{}).Name(); got != "qcow2" {
		t.Fatalf("Name() = %q, want %q", got, "qcow2")
	}
}

func TestFormatCreate_QCOW2_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := (Format{}).Create(path, 1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !IsQCOW2File(path) {
		t.Fatal("created file does not have QCOW2 magic")
	}
}

func TestFormatCreate_QCOW2_InvalidSize(t *testing.T) {
	if err := (Format{}).Create(filepath.Join(t.TempDir(), "x.qcow2"), -1); err == nil {
		t.Fatal("expected error for negative size")
	}
}

func TestFormatDetect_QCOW2_Valid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := Create(path, 1024*1024); err != nil {
		t.Fatal(err)
	}
	ok, err := (Format{}).Detect(path)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !ok {
		t.Fatal("Detect returned false for valid QCOW2 file")
	}
}

func TestFormatDetect_QCOW2_Invalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := os.WriteFile(path, make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, err := (Format{}).Detect(path)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ok {
		t.Fatal("Detect returned true for non-QCOW2 file")
	}
}

func TestFormatDetect_QCOW2_NotExist(t *testing.T) {
	ok, err := (Format{}).Detect(filepath.Join(t.TempDir(), "nofile"))
	if ok {
		t.Fatal("Detect returned true for non-existent path")
	}
	if err != nil {
		t.Fatalf("expected nil error from IsQCOW2File, got: %v", err)
	}
}

func TestFormatToRaw_QCOW2_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "disk.qcow2")
	dst := filepath.Join(dir, "disk.raw")
	const size = 1024 * 1024
	if err := Create(src, size); err != nil {
		t.Fatal(err)
	}
	if err := (Format{}).ToRaw(src, dst, io.Discard); err != nil {
		t.Fatalf("ToRaw: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if info.Size() != size {
		t.Errorf("raw size = %d, want %d", info.Size(), size)
	}
}

func TestFormatToRaw_QCOW2_BadSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst.raw")
	if err := (Format{}).ToRaw("/nonexistent.qcow2", dst, io.Discard); err == nil {
		t.Fatal("expected error for non-existent src")
	}
}

func TestFormatResize_QCOW2_InvalidSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := (Format{}).Resize(path, 0); err == nil {
		t.Fatal("expected error for size=0")
	}
}

func TestFormatResize_QCOW2_BadPath(t *testing.T) {
	if err := (Format{}).Resize("/nonexistent.qcow2", 512); err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestFormatResize_QCOW2_Grow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := (Format{}).Create(path, 1*1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := (Format{}).Resize(path, 2*1024*1024); err != nil {
		t.Fatalf("Resize grow: %v", err)
	}
	// Verify the new virtual size is stored in the header (offset 24, big-endian uint64).
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, 32)
	if _, err := f.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	got := uint64(buf[24])<<56 | uint64(buf[25])<<48 | uint64(buf[26])<<40 | uint64(buf[27])<<32 |
		uint64(buf[28])<<24 | uint64(buf[29])<<16 | uint64(buf[30])<<8 | uint64(buf[31])
	if got != 2*1024*1024 {
		t.Errorf("virtual size in header = %d, want %d", got, 2*1024*1024)
	}
}

func TestFormatResize_QCOW2_SameSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := (Format{}).Create(path, 1*1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := (Format{}).Resize(path, 1*1024*1024); err != nil {
		t.Fatalf("Resize same size: %v", err)
	}
}

func TestFormatResize_QCOW2_Shrink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := (Format{}).Create(path, 2*1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := (Format{}).Resize(path, 1*1024*1024); err == nil {
		t.Fatal("expected error when shrinking qcow2")
	}
}
