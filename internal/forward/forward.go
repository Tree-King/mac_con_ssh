package forward

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel/internal/config"
)

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
func (m *Manager) Serve(ctx context.Context, client *ssh.Client) error {
	var wg sync.WaitGroup
	errc := make(chan error, len(m.listeners))
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
				go handle(client, f, conn)
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
func handle(client *ssh.Client, f config.Forward, local net.Conn) {
	defer local.Close()
	remoteAddr := net.JoinHostPort(f.RemoteHost, fmt.Sprint(f.RemotePort))
	remote, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("forward %s failed to connect remote %s: %v", f.Name, remoteAddr, err)
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
