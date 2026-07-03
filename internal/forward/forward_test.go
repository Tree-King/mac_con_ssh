package forward

import (
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-tunnel/internal/config"
)

type failingDialer struct{ err error }

func (d failingDialer) Dial(string, string) (net.Conn, error) { return nil, d.err }

type delayedDialer struct {
	delays map[string]time.Duration
	mu     sync.Mutex
	conns  []*testConn
}

func (d *delayedDialer) Dial(_ string, addr string) (net.Conn, error) {
	time.Sleep(d.delays[addr])
	conn := &testConn{}
	d.mu.Lock()
	d.conns = append(d.conns, conn)
	d.mu.Unlock()
	return conn, nil
}

type testConn struct{ closed bool }

func (c *testConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *testConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *testConn) Close() error                     { c.closed = true; return nil }
func (c *testConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c *testConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c *testConn) SetDeadline(time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }

func TestHandleReportsDialFailure(t *testing.T) {
	local, peer := net.Pipe()
	defer peer.Close()

	errCh := make(chan error, 1)
	go handle(failingDialer{err: errors.New("can't assign requested address")}, config.Forward{
		Name:       "db",
		RemoteHost: "10.10.10.109",
		RemotePort: 50000,
	}, local, errCh)

	select {
	case err := <-errCh:
		got := err.Error()
		if !strings.Contains(got, "forward db failed to connect remote 10.10.10.109:50000") {
			t.Fatalf("error = %q, want forward and remote address context", got)
		}
		if !strings.Contains(got, "can't assign requested address") {
			t.Fatalf("error = %q, want original dial error", got)
		}
	case <-time.After(time.Second):
		t.Fatal("handle did not report dial failure")
	}
}

func TestDialBestRemoteChoosesLowestLatencyTarget(t *testing.T) {
	dialer := &delayedDialer{delays: map[string]time.Duration{
		"db-a.example.com:3306": 20 * time.Millisecond,
		"db-b.example.com:3306": 1 * time.Millisecond,
	}}
	fwd := config.Forward{
		Name: "db",
		RemoteTargets: []config.ForwardTarget{
			{Host: "db-a.example.com", Port: 3306},
			{Host: "db-b.example.com", Port: 3306},
		},
	}

	conn, addr, err := dialBestRemote(dialer, fwd)
	if err != nil {
		t.Fatalf("dialBestRemote returned error: %v", err)
	}
	defer conn.Close()
	if addr != "db-b.example.com:3306" {
		t.Fatalf("addr = %q, want db-b.example.com:3306", addr)
	}
	if len(dialer.conns) != 2 {
		t.Fatalf("dial count = %d, want 2", len(dialer.conns))
	}
	if !dialer.conns[0].closed {
		t.Fatal("slower first connection was not closed")
	}
	if dialer.conns[1].closed {
		t.Fatal("selected faster connection was closed before caller received it")
	}
}
