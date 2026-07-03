package forward

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

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
		log.Printf("local forward %s listening on %s -> %s:%d", f.Name, addr, f.RemoteHost, f.RemotePort)
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
	remoteAddr := net.JoinHostPort(f.RemoteHost, fmt.Sprint(f.RemotePort))
	remote, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		err = fmt.Errorf("forward %s failed to connect remote %s: %w", f.Name, remoteAddr, err)
		log.Print(err)
		select {
		case errc <- err:
		default:
		}
		return
	}
	defer remote.Close()
	log.Printf("forward %s established local %s -> remote %s", f.Name, local.RemoteAddr(), remoteAddr)
	done := make(chan struct{}, 2)
	go copyClose(done, remote, local)
	go copyClose(done, local, remote)
	<-done
}
func copyClose(done chan<- struct{}, dst net.Conn, src net.Conn) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	done <- struct{}{}
}
