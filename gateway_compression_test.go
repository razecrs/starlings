package starlings

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGatewayInflaterKeepsSharedContext(t *testing.T) {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	inflater := newGatewayInflater()
	defer inflater.Close()

	for _, payload := range []string{
		`{"op":10,"d":{"heartbeat_interval":45000}}`,
		`{"op":11,"d":null}`,
	} {
		start := compressed.Len()
		if _, err := zw.Write([]byte(payload)); err != nil {
			t.Fatal(err)
		}
		if err := zw.Flush(); err != nil {
			t.Fatal(err)
		}
		chunk := append([]byte(nil), compressed.Bytes()[start:]...)
		middle := len(chunk) / 2
		if err := inflater.Write(chunk[:middle]); err != nil {
			t.Fatal(err)
		}
		if err := inflater.Write(chunk[middle:]); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		got, err := inflater.Next(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != payload {
			t.Fatalf("payload = %s, want %s", got, payload)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayInflaterLimitsDecompressedFrames(t *testing.T) {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	payload := `{"op":0,"d":{"content":"` + strings.Repeat("a", 256) + `"}}`
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Flush(); err != nil {
		t.Fatal(err)
	}

	inflater := newGatewayInflaterLimit(64)
	defer inflater.Close()
	if err := inflater.Write(compressed.Bytes()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := inflater.Next(ctx)
	if !errors.Is(err, errGatewayFrameTooLarge) {
		t.Fatalf("oversized frame error = %v", err)
	}
}
