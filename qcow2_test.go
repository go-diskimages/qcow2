package image_qcow2

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── IsQCOW2File ─────────────────────────────────────────────────────────────

func TestIsQCOW2File_ValidMagic(t *testing.T) {
	data := make([]byte, 8)
	copy(data, []byte{0x51, 0x46, 0x49, 0xfb})
	path := filepath.Join(t.TempDir(), "test.qcow2")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if !IsQCOW2File(path) {
		t.Error("expected IsQCOW2File to return true for valid magic")
	}
}

func TestIsQCOW2File_BadMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.raw")
	if err := os.WriteFile(path, []byte("not a qcow2 file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if IsQCOW2File(path) {
		t.Error("expected IsQCOW2File to return false for bad magic")
	}
}

func TestIsQCOW2File_TooShort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short.bin")
	if err := os.WriteFile(path, []byte{0x51, 0x46}, 0o600); err != nil {
		t.Fatal(err)
	}
	if IsQCOW2File(path) {
		t.Error("expected IsQCOW2File to return false for file shorter than 4 bytes")
	}
}

func TestIsQCOW2File_Missing(t *testing.T) {
	if IsQCOW2File("/non/existent/file.qcow2") {
		t.Error("expected IsQCOW2File to return false for missing file")
	}
}

// ─── ConvertToRaw helpers ───────────────────────────────────────────────────

// makeMinimalQCOW2 builds a minimal valid QCOW2 v2 image containing a single
// 512-byte data cluster and writes it to a temp file, returning the path.
func makeMinimalQCOW2(t *testing.T, data []byte, compressed bool) string {
	t.Helper()
	const clusterBits = 9
	const clusterSize = 512
	img := make([]byte, 4*clusterSize)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 2)
	binary.BigEndian.PutUint32(img[20:24], clusterBits)
	binary.BigEndian.PutUint64(img[24:32], clusterSize)
	binary.BigEndian.PutUint32(img[36:40], 1)
	binary.BigEndian.PutUint64(img[40:48], 512)
	binary.BigEndian.PutUint64(img[512:520], 1024)
	if compressed {
		l2entry := uint64(1<<62) | uint64(1536)
		binary.BigEndian.PutUint64(img[1024:1032], l2entry)
		var buf bytes.Buffer
		fw, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		_, _ = fw.Write(data)
		_ = fw.Close()
		copy(img[1536:], buf.Bytes())
	} else {
		binary.BigEndian.PutUint64(img[1024:1032], 1536)
		copy(img[1536:], data)
	}
	path := filepath.Join(t.TempDir(), "test.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func progressNonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ─── ConvertToRaw ─────────────────────────────────────────────────────────────

func TestConvertToRaw_Uncompressed(t *testing.T) {
	data := bytes.Repeat([]byte{0xAB}, 512)
	src := makeMinimalQCOW2(t, data, false)
	dst := filepath.Join(t.TempDir(), "out.raw")
	var w bytes.Buffer
	if err := ConvertToRaw(src, dst, &w); err != nil {
		t.Fatalf("ConvertToRaw: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("output mismatch: got[0]=%02x, want[0]=%02x", got[0], data[0])
	}
}

func TestConvertToRaw_Compressed(t *testing.T) {
	data := bytes.Repeat([]byte{0xCD}, 512)
	src := makeMinimalQCOW2(t, data, true)
	dst := filepath.Join(t.TempDir(), "out.raw")
	var w bytes.Buffer
	if err := ConvertToRaw(src, dst, &w); err != nil {
		t.Fatalf("ConvertToRaw compressed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("compressed output mismatch: got[0]=%02x, want[0]=%02x", got[0], data[0])
	}
}

func TestConvertToRaw_CompressedIncompressible(t *testing.T) {
	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i * 7)
	}
	src := makeMinimalQCOW2(t, data, true)
	dst := filepath.Join(t.TempDir(), "out.raw")
	var w bytes.Buffer
	if err := ConvertToRaw(src, dst, &w); err != nil {
		t.Fatalf("ConvertToRaw incompressible: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, data) {
		t.Errorf("incompressible compressed: output mismatch")
	}
}

func TestConvertToRaw_UnallocatedL2Entry(t *testing.T) {
	const clusterBits = 9
	const clusterSize = 512
	img := make([]byte, 3*clusterSize)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 2)
	binary.BigEndian.PutUint32(img[20:24], clusterBits)
	binary.BigEndian.PutUint64(img[24:32], clusterSize)
	binary.BigEndian.PutUint32(img[36:40], 1)
	binary.BigEndian.PutUint64(img[40:48], 512)
	binary.BigEndian.PutUint64(img[512:520], 1024)
	// L2 entry is zero → unallocated cluster → should read as zeros
	path := filepath.Join(t.TempDir(), "zero.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out.raw")
	if err := ConvertToRaw(path, dst, io.Discard); err != nil {
		t.Fatalf("unallocated L2 entry: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, make([]byte, clusterSize)) {
		t.Error("expected all-zero output for unallocated cluster")
	}
}

func TestConvertToRaw_UnallocatedL1Entry(t *testing.T) {
	const clusterBits = 9
	const clusterSize = 512
	img := make([]byte, 2*clusterSize)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 2)
	binary.BigEndian.PutUint32(img[20:24], clusterBits)
	binary.BigEndian.PutUint64(img[24:32], clusterSize)
	binary.BigEndian.PutUint32(img[36:40], 1)
	binary.BigEndian.PutUint64(img[40:48], 512)
	// L1 entry is zero → no L2 table → should read as zeros
	path := filepath.Join(t.TempDir(), "nol2.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out.raw")
	if err := ConvertToRaw(path, dst, io.Discard); err != nil {
		t.Fatalf("unallocated L1 entry: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, make([]byte, clusterSize)) {
		t.Error("expected all-zero output for unallocated L1 entry")
	}
}

func TestConvertToRaw_ProgressLines(t *testing.T) {
	data := bytes.Repeat([]byte{0x01}, 512)
	src := makeMinimalQCOW2(t, data, false)
	dst := filepath.Join(t.TempDir(), "out.raw")
	var w bytes.Buffer
	if err := ConvertToRaw(src, dst, &w); err != nil {
		t.Fatalf("progress test: %v", err)
	}
	lines := progressNonEmptyLines(w.String())
	if len(lines) == 0 {
		t.Fatal("no progress lines emitted")
	}
	last := lines[len(lines)-1]
	if last != "100%" {
		t.Errorf("last progress line = %q, want \"100%%\"", last)
	}
}

func TestConvertToRaw_ErrorBadMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.qcow2")
	if err := os.WriteFile(path, make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	err := ConvertToRaw(path, filepath.Join(t.TempDir(), "out.raw"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "magic") {
		t.Errorf("expected bad magic error, got: %v", err)
	}
}

func TestConvertToRaw_ErrorBadVersion(t *testing.T) {
	img := make([]byte, 512)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 4)
	path := filepath.Join(t.TempDir(), "v4.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	err := ConvertToRaw(path, filepath.Join(t.TempDir(), "out.raw"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("expected version error, got: %v", err)
	}
}

func TestConvertToRaw_ErrorEncrypted(t *testing.T) {
	img := make([]byte, 512)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 2)
	binary.BigEndian.PutUint32(img[20:24], 9)
	binary.BigEndian.PutUint64(img[24:32], 512)
	binary.BigEndian.PutUint32(img[32:36], 1)
	path := filepath.Join(t.TempDir(), "enc.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	err := ConvertToRaw(path, filepath.Join(t.TempDir(), "out.raw"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected encryption error, got: %v", err)
	}
}

func TestConvertToRaw_ErrorInvalidClusterBits(t *testing.T) {
	img := make([]byte, 512)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 2)
	binary.BigEndian.PutUint32(img[20:24], 0)
	path := filepath.Join(t.TempDir(), "badcb.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	err := ConvertToRaw(path, filepath.Join(t.TempDir(), "out.raw"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "cluster_bits") {
		t.Errorf("expected cluster_bits error, got: %v", err)
	}
}

func TestConvertToRaw_MissingFile(t *testing.T) {
	err := ConvertToRaw("/non/existent/file.qcow2", filepath.Join(t.TempDir(), "out.raw"), io.Discard)
	if err == nil {
		t.Error("expected error for missing source file")
	}
}

func TestConvertToRaw_ErrorInvalidVirtualSize(t *testing.T) {
	img := make([]byte, 512)
	copy(img[0:4], []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(img[4:8], 2)
	binary.BigEndian.PutUint32(img[20:24], 9)
	binary.BigEndian.PutUint64(img[24:32], 0) // virtualSize = 0 → error
	path := filepath.Join(t.TempDir(), "zerovirt.qcow2")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	err := ConvertToRaw(path, filepath.Join(t.TempDir(), "out.raw"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "virtual size") {
		t.Errorf("expected virtual size error, got: %v", err)
	}
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestCreate_ValidMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.qcow2")
	if err := Create(path, 1<<20 /* 1 MiB */); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !IsQCOW2File(path) {
		t.Error("created file not recognised as QCOW2 by IsQCOW2File")
	}
}

func TestCreate_ConvertRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.qcow2")
	if err := Create(src, 512); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dst := filepath.Join(dir, "out.raw")
	if err := ConvertToRaw(src, dst, io.Discard); err != nil {
		t.Fatalf("ConvertToRaw: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) != 512 {
		t.Fatalf("raw size = %d, want 512", len(data))
	}
	for i, b := range data {
		if b != 0 {
			t.Fatalf("raw byte[%d] = %d, want 0", i, b)
		}
	}
}

func TestCreate_InvalidSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.qcow2")
	if err := Create(path, 0); err == nil {
		t.Fatal("expected error for size=0")
	}
	if err := Create(path, -1); err == nil {
		t.Fatal("expected error for size=-1")
	}
}

func TestCreate_BadPath(t *testing.T) {
	err := Create("/nonexistent/dir/disk.qcow2", 1<<20)
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}
