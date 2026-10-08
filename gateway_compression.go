package starlings

import (
	"compress/zlib"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"sync"
)

// gatewayInflater owns the shared zlib context Discord requires for one
// gateway connection. Z_SYNC_FLUSH ends a payload, not the zlib stream, so a
// fresh zlib.Reader per WebSocket message would fail after the first event.
type gatewayInflater struct {
	input  *io.PipeWriter
	frames chan []byte
	errors chan error
	once   sync.Once
}

var errGatewayFrameTooLarge = errors.New("starlings: decompressed gateway frame exceeds limit")

// gatewayFrameReader applies the frame limit after decompression. The
// WebSocket read limit only constrains compressed bytes, so it cannot protect
// against a small zlib payload expanding to an unreasonable size.
type gatewayFrameReader struct {
	r     io.Reader
	limit int64
	read  int64
}

func (r *gatewayFrameReader) Read(p []byte) (int, error) {
	remaining := r.limit - r.read
	if remaining <= 0 {
		return 0, errGatewayFrameTooLarge
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	return n, err
}

func (r *gatewayFrameReader) reset() { r.read = 0 }

func newGatewayInflater() *gatewayInflater {
	return newGatewayInflaterLimit(readLimit)
}

func newGatewayInflaterLimit(limit int64) *gatewayInflater {
	reader, writer := io.Pipe()
	i := &gatewayInflater{
		input:  writer,
		frames: make(chan []byte, 1),
		errors: make(chan error, 1),
	}
	go i.inflate(reader, limit)
	return i
}

func (i *gatewayInflater) inflate(input *io.PipeReader, limit int64) {
	defer input.Close()
	zr, err := zlib.NewReader(input)
	if err != nil {
		i.report(err)
		return
	}
	defer zr.Close()

	limited := &gatewayFrameReader{r: zr, limit: limit}
	decoder := jsontext.NewDecoder(limited)
	for {
		value, err := decoder.ReadValue()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				i.report(fmt.Errorf("decompressing gateway payload: %w", err))
			}
			return
		}
		limited.reset()
		frame := append([]byte(nil), value...)
		if len(frame) > largeFrame {
			// The decoder keeps a buffer as large as the largest frame it has
			// read, for the life of the connection. Replace it after a large
			// frame such as a big GUILD_CREATE. Nothing of the next frame is
			// buffered yet: it is only written after this one is taken.
			decoder = jsontext.NewDecoder(limited)
		}
		i.frames <- frame
	}
}

func (i *gatewayInflater) report(err error) {
	select {
	case i.errors <- err:
	default:
	}
}

func (i *gatewayInflater) Write(data []byte) error {
	_, err := i.input.Write(data)
	return err
}

func (i *gatewayInflater) Next(ctx context.Context) ([]byte, error) {
	select {
	case frame := <-i.frames:
		return frame, nil
	case err := <-i.errors:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (i *gatewayInflater) Close() {
	i.once.Do(func() { _ = i.input.Close() })
}

func (g *gateway) read(ctx context.Context) ([]byte, error) {
	if g.inflater == nil {
		_, data, err := g.conn.Read(ctx)
		return data, err
	}

	for {
		select {
		case frame := <-g.inflater.frames:
			return frame, nil
		case err := <-g.inflater.errors:
			return nil, err
		default:
		}

		_, compressed, err := g.conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		if err := g.inflater.Write(compressed); err != nil {
			return nil, err
		}
		if len(compressed) >= 4 && compressed[len(compressed)-4] == 0 && compressed[len(compressed)-3] == 0 && compressed[len(compressed)-2] == 0xff && compressed[len(compressed)-1] == 0xff {
			return g.inflater.Next(ctx)
		}
	}
}
