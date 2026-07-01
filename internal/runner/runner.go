package runner

import (
	"context"
	"log"
	"time"

	"ssh-tunnel/internal/config"
	"ssh-tunnel/internal/forward"
	"ssh-tunnel/internal/sshclient"
)

func Run(ctx context.Context, name string, server config.Server) error {
	attempt := 0
	delay := time.Duration(server.Reconnect.InitialDelaySeconds) * time.Second
	maxDelay := time.Duration(server.Reconnect.MaxDelaySeconds) * time.Second
	for {
		client, err := sshclient.Connect(name, server)
		if err != nil {
			attempt++
			if !server.Reconnect.Enabled || (server.Reconnect.MaxRetries > 0 && attempt > server.Reconnect.MaxRetries) {
				return err
			}
			log.Printf("connect failed, retrying in %s: %v", delay, err)
			if !sleep(ctx, delay) {
				return ctx.Err()
			}
			delay = nextDelay(delay, maxDelay)
			continue
		}
		attempt = 0
		delay = time.Duration(server.Reconnect.InitialDelaySeconds) * time.Second
		sessionCtx, cancel := context.WithCancel(ctx)
		mgr := forward.New(server.Forwards)
		if err := mgr.ListenAll(); err != nil {
			cancel()
			_ = client.Close()
			return err
		}
		done := make(chan error, 1)
		if server.Keepalive.Enabled {
			go sshclient.StartKeepalive(sessionCtx.Done(), client, time.Duration(server.Keepalive.IntervalSeconds)*time.Second)
		}
		go func() { done <- mgr.Serve(sessionCtx, client) }()
		err = <-done
		cancel()
		mgr.Close()
		_ = client.Close()
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("SSH session ended: %v", err)
		if !server.Reconnect.Enabled {
			return err
		}
		log.Printf("reconnecting in %s", delay)
		if !sleep(ctx, delay) {
			return ctx.Err()
		}
		delay = nextDelay(delay, maxDelay)
	}
}
func nextDelay(current, max time.Duration) time.Duration {
	current *= 2
	if current > max {
		return max
	}
	return current
}
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
