package git

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestHeaderScanning(t *testing.T) {
	scanner := NewHeaderScanner(strings.NewReader("commit 262\x00"))

	header, _, err := scanner.Scan()
	if err != nil {
		t.Fatal(err.Error())
	}

	expectedKind, actualKind := "commit", header.Kind
	expectedSize, actualSize := int64(262), header.Size

	if actualKind != expectedKind {
		t.Fatalf("incorrect header kind. expetced %s, got %s", expectedKind, actualKind)
	}

	if actualSize != expectedSize {
		t.Fatalf("incorrect header size. expetced %d, got %d", 262, header.Size)
	}
}

func TestBufferOverwrite(t *testing.T) {
	scanner := NewHeaderScanner(strings.NewReader("commit 262\x00Hello, world!"))

	_, payload, err := scanner.Scan()
	if err != nil {
		t.Fatal(err.Error())
	}

	scanner.Reset(strings.NewReader("blob 4052\x00Wait, this isn't right..."))

	_, otherPayload, err := scanner.Scan()
	if err != nil {
		t.Fatal(err.Error())
	}

	payloadBytes, err := io.ReadAll(payload)
	if err != nil {
		t.Fatal(err.Error())
	}

	payloadString := string(payloadBytes)
	if payloadString != "Hello, world!" {
		t.Fatal("payload currupted")
	}

	otherPayloadBytes, err := io.ReadAll(otherPayload)
	if err != nil {
		t.Fatal(err.Error())
	}
	otherPayloadString := string(otherPayloadBytes)
	if otherPayloadString != "Wait, this isn't right..." {
		t.Fatal("payload corrupted")
	}
}

func TestNoReset(t *testing.T) {
	scanner := NewHeaderScanner(strings.NewReader("commit 262\x00Hello, world!"))

	_, _, _ = scanner.Scan()

	_, _, err := scanner.Scan()
	if !errors.Is(err, ErrNotReset) {
		t.Fatalf("error should be ErrNotReset but is %s", err.Error())
	}
}

const obj = "commit 32\x00abcdefghijklmnopqrstuvwxyzabcdef" // 32 bytes payload

func BenchmarkScanner_Reset(b *testing.B) {
	b.ReportAllocs()
	hs := NewHeaderScanner(strings.NewReader(obj)) // initial, will be Reset()’d
	for i := 0; i < b.N; i++ {
		hs.Reset(strings.NewReader(obj))
		_, r, err := hs.Scan()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			b.Fatal(err)
		}
	}
}
