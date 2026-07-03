package forward

import (
	"errors"
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
