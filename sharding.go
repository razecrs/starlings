package starlings

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const identifyWindow = 5 * time.Second

func (c *Client) runAutoSharded(ctx context.Context) error {
	// Most bots need one shard at the usual address, so connect to it while
	// asking Discord how many shards to use. The connection is only
	// identified on once the answer confirms it.
	pre := c.startPreDial(ctx, defaultGatewayURL)
	info, err := c.GatewayBot(ctx)
	if err != nil {
		pre.discard()
		return fmt.Errorf("looking up gateway shards: %w", err)
	}
	count := max(info.Shards, 1)
	c.gatewayBase = info.URL
	if count == 1 {
		c.gw.pre = pre
		return c.gw.run(ctx, c)
	}
	pre.discard()
	if info.SessionStartLimit.Remaining < count {
		return fmt.Errorf("starlings: Discord recommends %d shards but only %d identify sessions remain", count, info.SessionStartLimit.Remaining)
	}

	shards := c.makeShards(count, info.URL)
	concurrency := max(info.SessionStartLimit.MaxConcurrency, 1)
	c.log.Info("starlings: starting gateway shards", "count", count, "max_concurrency", concurrency)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, count)
	var wg sync.WaitGroup

	for start := 0; start < count; start += concurrency {
		end := min(start+concurrency, count)
		for _, shard := range shards[start:end] {
			wg.Add(1)
			go func(shard *Client) {
				defer wg.Done()
				if err := shard.gw.run(runCtx, shard); err != nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
				}
			}(shard)
		}
		if end < count {
			timer := time.NewTimer(identifyWindow)
			select {
			case err := <-errCh:
				timer.Stop()
				wg.Wait()
				return err
			case <-ctx.Done():
				timer.Stop()
				wg.Wait()
				return nil
			case <-timer.C:
			}
		}
	}

	select {
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	case <-ctx.Done():
		cancel()
		wg.Wait()
		return nil
	}
}

func (c *Client) makeShards(count int, gatewayURL string) []*Client {
	if count < 1 {
		count = 1
	}
	c.shard = [2]int{0, count}
	c.gatewayBase = gatewayURL
	shards := make([]*Client, count)
	shards[0] = c
	for id := 1; id < count; id++ {
		child := &Client{
			token: c.token, id: c.id,
			intents: c.intents, log: c.log, rest: c.rest,
			asyncEvents: c.asyncEvents, guard: c.guard,
			recoverPanics: c.recoverPanics, autoDefer: c.autoDefer, trimMemory: c.trimMemory,
			requestTimeout: c.requestTimeout, intentsSet: c.intentsSet,
			shard: [2]int{id, count}, compress: c.compress,
			gatewayBase: gatewayURL, initialState: c.initialState,
			State: c.State, prefix: c.prefix,
			cmds: c.cmds, cmdHooked: c.cmdHooked,
			slashes: c.slashes, slashHooked: c.slashHooked,
			ready: make(chan struct{}), shardRoot: c,
		}
		child.slots.Store(c.slots.Load())
		shards[id] = child
	}
	c.shardsMu.Lock()
	c.shards = shards
	c.shardsMu.Unlock()
	return shards
}

// ShardCount returns the number of gateway shards currently configured.
func (c *Client) ShardCount() int {
	root := c.rootClient()
	root.shardsMu.RLock()
	count := len(root.shards)
	root.shardsMu.RUnlock()
	if count != 0 {
		return count
	}
	if root.shard[1] > 0 {
		return root.shard[1]
	}
	return 1
}

// ShardStatus is a concurrency-safe snapshot of one gateway connection.
type ShardStatus struct {
	ID        int
	Count     int
	Connected bool
	// Ready reports that this connection has received READY or RESUMED and
	// is delivering events. It becomes false again when the connection drops.
	Ready     bool
	Latency   time.Duration
	Sequence  int64
	Resumable bool
}

// ShardStatuses returns a stable snapshot suitable for dashboards and health
// endpoints. It never exposes the live gateway objects.
func (c *Client) ShardStatuses() []ShardStatus {
	root := c.rootClient()
	root.shardsMu.RLock()
	clients := append([]*Client(nil), root.shards...)
	root.shardsMu.RUnlock()
	if len(clients) == 0 {
		clients = []*Client{root}
	}
	out := make([]ShardStatus, len(clients))
	for i, shard := range clients {
		count := shard.shard[1]
		if count < 1 {
			count = len(clients)
		}
		out[i] = ShardStatus{
			ID:        shard.shard[0],
			Count:     count,
			Connected: shard.gw.connected.Load(),
			Ready:     shard.gw.live.Load(),
			Latency:   time.Duration(shard.gw.latency.Load()),
			Sequence:  shard.seq.Load(),
			Resumable: shard.sessionID.Load() != nil,
		}
	}
	return out
}

// Online reports whether every shard currently has a ready session. Unlike
// WaitReady, which only waits for the first READY, it becomes false while any
// shard is disconnected or still identifying.
func (c *Client) Online() bool {
	for _, status := range c.ShardStatuses() {
		if !status.Ready {
			return false
		}
	}
	return true
}

// ShardIDForGuild reports which shard owns a guild using Discord's official
// snowflake routing formula.
func (c *Client) ShardIDForGuild(guildID Snowflake) int {
	count := c.ShardCount()
	return int((uint64(guildID) >> 22) % uint64(count))
}

func (c *Client) rootClient() *Client {
	if c.shardRoot != nil {
		return c.shardRoot
	}
	return c
}

func (c *Client) gatewayForGuild(guildID Snowflake) (*gateway, error) {
	root := c.rootClient()
	root.shardsMu.RLock()
	defer root.shardsMu.RUnlock()
	if len(root.shards) == 0 {
		return &root.gw, nil
	}
	id := int((uint64(guildID) >> 22) % uint64(len(root.shards)))
	return &root.shards[id].gw, nil
}

func (c *Client) gateways() []*gateway {
	root := c.rootClient()
	root.shardsMu.RLock()
	defer root.shardsMu.RUnlock()
	if len(root.shards) == 0 {
		return []*gateway{&root.gw}
	}
	out := make([]*gateway, len(root.shards))
	for i, shard := range root.shards {
		out[i] = &shard.gw
	}
	return out
}

func sendAll(ctx context.Context, gateways []*gateway, op Opcode, data any) error {
	var errs []error
	for _, gateway := range gateways {
		if err := gateway.send(ctx, op, data); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
