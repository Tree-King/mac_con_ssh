package forward

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"ssh-tunnel/internal/config"
)

type failingDialer struct{ err error }

func (d failingDialer) Dial(string, string) (net.Conn, error) { return nil, d.err }

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
		if !strings.Contains(got, "forward db failed to connect target 10.10.10.109:50000") {
			t.Fatalf("error = %q, want forward and remote address context", got)
		}
		if !strings.Contains(got, "can't assign requested address") {
			t.Fatalf("error = %q, want original dial error", got)
		}
	case <-time.After(time.Second):
		t.Fatal("handle did not report dial failure")
	}
}

func TestDialBestDirectChoosesReachableTarget(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	fwd := config.Forward{
		Name: "db",
		DirectTargets: []config.ForwardTarget{
			{Host: "127.0.0.2", Port: port},
			{Host: "127.0.0.1", Port: port},
		},
	}

	conn, addr, err := dialBestDirect(fwd)
	if err != nil {
		t.Fatalf("dialBestDirect returned error: %v", err)
	}
	defer conn.Close()
	want := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
	if addr != want {
		t.Fatalf("addr = %q, want %s", addr, want)
	}
}
