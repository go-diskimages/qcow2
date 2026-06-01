package image_qcow2

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// createTestQCOW2 creates a small temporary qcow2 image and returns its path.
func createTestQCOW2(t *testing.T, sizeBytes int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.qcow2")
	if err := Create(path, sizeBytes); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return path
}

// ─── OpenDevice errors ────────────────────────────────────────────────────────

func TestOpenDevice_MissingFile(t *testing.T) {
	_, err := OpenDevice("/nonexistent/path.qcow2")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestOpenDevice_BadMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.bin")
	if err := os.WriteFile(path, make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenDevice(path)
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
}

func TestOpenDevice_TooShort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short.bin")
	if err := os.WriteFile(path, []byte{0x51, 0x46, 0x49, 0xfb}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenDevice(path)
	if err == nil {
		t.Fatal("expected error for file shorter than 48 bytes")
	}
}

// ─── Size ────────────────────────────────────────────────────────────────────

func TestDevice_Size(t *testing.T) {
	const want = 4 * 1024 * 1024 // 4 MiB
	path := createTestQCOW2(t, want)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()
	got, err := d.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if got != want {
		t.Errorf("Size = %d, want %d", got, want)
	}
}

// ─── ReadAt unallocated ───────────────────────────────────────────────────────

func TestDevice_ReadAt_Unallocated_ReturnsZeros(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()

	buf := make([]byte, 512)
	n, err := d.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != 512 {
		t.Errorf("ReadAt returned %d bytes, want 512", n)
	}
	if !bytes.Equal(buf, make([]byte, 512)) {
		t.Error("ReadAt on unallocated cluster should return zeros")
	}
}

// ─── ReadAt past end ──────────────────────────────────────────────────────────

func TestDevice_ReadAt_PastEnd(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()

	buf := make([]byte, 16)
	_, err = d.ReadAt(buf, 4*1024*1024)
	if err == nil {
		t.Fatal("expected EOF for read at virtual size boundary")
	}
}

// ─── WriteAt / ReadAt round-trip ──────────────────────────────────────────────

func TestDevice_WriteRead_Roundtrip(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()

	want := []byte("hello qcow2 device test payload!")
	off := int64(512)
	if _, err := d.WriteAt(want, off); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	got := make([]byte, len(want))
	if _, err := d.ReadAt(got, off); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("ReadAt = %q, want %q", got, want)
	}
}

// ─── WriteAt spanning cluster boundary ───────────────────────────────────────

func TestDevice_WriteRead_MultiCluster(t *testing.T) {
	const clusterSize = 1 << 16 // 64 KiB (cluster_bits=16 used by Create)
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()

	// Write data that straddles the boundary between cluster 0 and cluster 1.
	off := int64(clusterSize - 8)
	want := make([]byte, 16)
	for i := range want {
		want[i] = byte(i + 1)
	}
	if _, err := d.WriteAt(want, off); err != nil {
		t.Fatalf("WriteAt (multi-cluster): %v", err)
	}
	got := make([]byte, len(want))
	if _, err := d.ReadAt(got, off); err != nil {
		t.Fatalf("ReadAt (multi-cluster): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("multi-cluster: got %v, want %v", got, want)
	}
}

// ─── WriteAt out of range ─────────────────────────────────────────────────────

func TestDevice_WriteAt_OutOfRange(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()

	_, err = d.WriteAt([]byte("x"), 4*1024*1024)
	if err == nil {
		t.Fatal("expected error for write at virtual size boundary")
	}
}

// ─── Sync ─────────────────────────────────────────────────────────────────────

func TestDevice_Sync(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

// ─── Truncate not supported ───────────────────────────────────────────────────

func TestDevice_Truncate_NotSupported(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)
	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	defer d.Close()
	if err := d.Truncate(8 * 1024 * 1024); err == nil {
		t.Fatal("expected error from Truncate")
	}
}

// ─── Persistence across close/reopen ─────────────────────────────────────────

func TestDevice_Persist_Across_Reopen(t *testing.T) {
	path := createTestQCOW2(t, 4*1024*1024)

	want := []byte("persist across reopen")
	off := int64(1024)

	d, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	if _, err := d.WriteAt(want, off); err != nil {
		d.Close()
		t.Fatalf("WriteAt: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	d2, err := OpenDevice(path)
	if err != nil {
		t.Fatalf("OpenDevice (reopen): %v", err)
	}
	defer d2.Close()

	got := make([]byte, len(want))
	if _, err := d2.ReadAt(got, off); err != nil {
		t.Fatalf("ReadAt (reopen): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("after reopen: got %q, want %q", got, want)
	}
}
