package forward

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"ssh-tunnel/internal/config"
)

type dialer interface {
	Dial(network, addr string) (net.Conn, error)
}

type Manager struct {
	Forwards  []config.Forward
	listeners []net.Listener
}

func New(forwards []config.Forward) *Manager { return &Manager{Forwards: forwards} }
func (m *Manager) ListenAll() error {
	for _, f := range m.Forwards {
		addr := net.JoinHostPort(f.LocalHost, fmt.Sprint(f.LocalPort))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			m.Close()
			return fmt.Errorf("listen %s for forward %s: %w", addr, f.Name, err)
		}
		log.Printf("local forward %s listening on %s -> %s", f.Name, addr, formatForwardDestination(f))
		m.listeners = append(m.listeners, ln)
	}
	return nil
}
func (m *Manager) Serve(ctx context.Context, client dialer) error {
	var wg sync.WaitGroup
	errc := make(chan error, len(m.listeners)+1)
	for i, ln := range m.listeners {
		f := m.Forwards[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				conn, err := ln.Accept()
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
						errc <- err
						return
					}
				}
				go handle(client, f, conn, errc)
			}
		}()
	}
	select {
	case <-ctx.Done():
		m.Close()
		wg.Wait()
		return nil
	case err := <-errc:
		m.Close()
		wg.Wait()
		return err
	}
}
func (m *Manager) Close() {
	for _, ln := range m.listeners {
		_ = ln.Close()
	}
}
func handle(client dialer, f config.Forward, local net.Conn, errc chan<- error) {
	defer local.Close()
	remote, remoteAddr, err := dialForwardTarget(client, f)
	if err != nil {
		err = fmt.Errorf("forward %s failed to connect target %s: %w", f.Name, formatForwardDestination(f), err)
		log.Print(err)
		select {
		case errc <- err:
		default:
		}
		return
	}
	defer remote.Close()
	log.Printf("forward %s established local %s -> target %s", f.Name, local.RemoteAddr(), remoteAddr)
	done := make(chan struct{}, 2)
	go copyClose(done, remote, local)
	go copyClose(done, local, remote)
	<-done
}

func dialForwardTarget(client dialer, f config.Forward) (net.Conn, string, error) {
	if len(f.DirectTargets) > 0 {
		return dialBestDirect(f)
	}
	addr := net.JoinHostPort(f.RemoteHost, fmt.Sprint(f.RemotePort))
	conn, err := client.Dial("tcp", addr)
	return conn, addr, err
}

func dialBestDirect(f config.Forward) (net.Conn, string, error) {
	targets := f.DirectCandidates()
	if len(targets) == 0 {
		return nil, "", fmt.Errorf("no direct targets configured")
	}
	dialer := &net.Dialer{}
	if len(targets) == 1 {
		addr := net.JoinHostPort(targets[0].Host, fmt.Sprint(targets[0].Port))
		conn, err := dialer.Dial("tcp", addr)
		return conn, addr, err
	}

	var best net.Conn
	bestAddr := ""
	var bestLatency time.Duration
	var errs []error
	for _, target := range targets {
		addr := net.JoinHostPort(target.Host, fmt.Sprint(target.Port))
		start := time.Now()
		conn, err := dialer.Dial("tcp", addr)
		latency := time.Since(start)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", addr, err))
			log.Printf("forward %s direct candidate %s unreachable after %s: %v", f.Name, addr, latency.Round(time.Millisecond), err)
			continue
		}
		log.Printf("forward %s direct candidate %s connected in %s", f.Name, addr, latency.Round(time.Millisecond))
		if best == nil || latency < bestLatency {
			if best != nil {
				_ = best.Close()
			}
			best = conn
			bestAddr = addr
			bestLatency = latency
			continue
		}
		_ = conn.Close()
	}
	if best == nil {
		return nil, "", fmt.Errorf("all direct targets failed: %v", errs)
	}
	log.Printf("forward %s selected direct target %s", f.Name, bestAddr)
	return best, bestAddr, nil
}

func formatForwardDestination(f config.Forward) string {
	if len(f.DirectTargets) > 0 {
		return formatTargets(f.DirectCandidates())
	}
	return net.JoinHostPort(f.RemoteHost, fmt.Sprint(f.RemotePort))
}

func formatTargets(targets []config.ForwardTarget) string {
	addrs := make([]string, 0, len(targets))
	for _, target := range targets {
		addrs = append(addrs, net.JoinHostPort(target.Host, fmt.Sprint(target.Port)))
	}
	return strings.Join(addrs, ", ")
}
func copyClose(done chan<- struct{}, dst net.Conn, src net.Conn) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	done <- struct{}{}
}
