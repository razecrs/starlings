package starlings

import (
	"context"
	"errors"
	"time"
)

// Extension returns the value stored under key, calling create to make it on
// first use. Every shard of an auto-sharded client shares the same value.
//
// Packages that add optional features, such as voice, use it to keep one
// manager per client without the core package importing them. Use an
// unexported key type so values from different packages cannot collide.
//
// When the client closes, a stored value that implements
// interface{ Close(context.Context) } is closed with a five-second deadline.
// If create returns an error, nothing is stored and the next call tries again.
func (c *Client) Extension(key any, create func() (any, error)) (any, error) {
	if key == nil || create == nil {
		return nil, errors.New("starlings: Extension needs a key and a create function")
	}
	root := c.rootClient()
	root.extMu.Lock()
	defer root.extMu.Unlock()
	if v, ok := root.ext[key]; ok {
		return v, nil
	}
	v, err := create()
	if err != nil {
		return nil, err
	}
	if root.ext == nil {
		root.ext = make(map[any]any)
	}
	root.ext[key] = v
	return v, nil
}

func (c *Client) closeExtensions() {
	root := c.rootClient()
	root.extMu.Lock()
	values := make([]any, 0, len(root.ext))
	for _, v := range root.ext {
		values = append(values, v)
	}
	root.extMu.Unlock()

	for _, v := range values {
		if closer, ok := v.(interface{ Close(context.Context) }); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			closer.Close(ctx)
			cancel()
		}
	}
}

// OnOrdered registers a handler that runs on the gateway goroutine, in event
// order, after the state cache and before application handlers. It keeps that
// order even with WithAsyncEvents.
//
// It exists for packages that track gateway state, such as voice. The handler
// must return quickly: while it runs, the shard reads no further events.
// Ordered handlers stay registered for the life of the client.
func OnOrdered[E Event](c *Client, h func(*E)) {
	registerEvent(c, h, true)
}
