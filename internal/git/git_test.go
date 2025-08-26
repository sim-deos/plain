package git

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"testing"
)

const (
	// Complex commit object for full decoding benchmarks
	testCommitObj = "commit 1234\x00" +
		`tree abcdef1234567890abcdef1234567890abcdef12
parent def0123456789abcdef0123456789abcdef0123
parent fedcba0987654321fedcba0987654321fedcba09
author John Doe <john.doe@example.com> 1703123456 +0000
committer Jane Smith <jane.smith@example.com> 1703123457 +0000

This is a test commit message with some content.
It has multiple lines and should be realistic.`
)

// createCompressedBuffer creates a buffer containing compressed data from the given content
func createCompressedBuffer(content string) *bytes.Buffer {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	io.WriteString(zw, content)
	zw.Close()
	return &buf
}

func TestHeaderScanning(t *testing.T) {
	// Use the helper function instead of manual buffer creation
	buf := createCompressedBuffer("commit 262\x00")

	// Now buf contains compressed data - pass the buffer directly to NewDecoder
	d, err := NewDecoder(buf)
	if err != nil {
		t.Fatal(err.Error())
	}

	header, err := d.Header()
	if err != nil {
		t.Fatal(err.Error())
	}

	expectedKind, actualKind := CommitObject, header.Kind
	expectedSize, actualSize := int64(262), header.Size

	if actualKind != expectedKind {
		t.Fatalf("incorrect header kind. expetced %s, got %s", expectedKind, actualKind)
	}

	if actualSize != expectedSize {
		t.Fatalf("incorrect header size. expetced %d, got %d", 262, header.Size)
	}
}

func TestNoReset(t *testing.T) {
	// Use the helper function instead of manual buffer creation
	buf := createCompressedBuffer("commit 262\x00\nHello, world!")

	d, err := NewDecoder(buf)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = d.Header()

	_, err = d.Header()
	if !errors.Is(err, ErrNotReset) {
		t.Fatalf("error should be ErrNotReset but is %s", err.Error())
	}
}

func BenchmarkScanner_Reset(b *testing.B) {
	b.ReportAllocs()

	// Create compressed data once outside the benchmark loop
	compressedData := createCompressedBuffer(testCommitObj)
	compressedBytes := compressedData.Bytes()

	d, _ := NewDecoder(bytes.NewReader(compressedBytes))
	defer d.Close()
	for b.Loop() {
		// Create a new reader with the same data for each iteration
		// This simulates real-world usage where you'd have different sources
		d.Reset(bytes.NewReader(compressedBytes))
		_, err := d.Header()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCommitDecoding(b *testing.B) {
	b.ReportAllocs()

	// Create compressed data once outside the benchmark loop
	compressedData := createCompressedBuffer(testCommitObj)
	compressedBytes := compressedData.Bytes()

	d, _ := NewDecoder(bytes.NewReader(compressedBytes))
	defer d.Close()
	for b.Loop() {
		// Create a new reader with the same data for each iteration
		d.Reset(bytes.NewReader(compressedBytes))

		// Decode the commit directly (this will also parse the header internally)
		_, err := d.DecodeCommit("test-hash-123")
		if err != nil {
			b.Fatal(err)
		}
	}
}
