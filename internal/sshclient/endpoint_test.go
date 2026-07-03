package sshclient

import (
	"net"
	"testing"

	"ssh-tunnel/internal/config"
)

func TestSelectBestEndpointReturnsOnlyHostWithoutProbeDial(t *testing.T) {
	server := config.Server{Host: "example.com", Port: 22}
	host, probes, err := SelectBestEndpoint(server)
	if err != nil {
		t.Fatalf("SelectBestEndpoint returned error: %v", err)
	}
	if host != "example.com" {
		t.Fatalf("host = %q, want example.com", host)
	}
	if len(probes) != 1 || probes[0].Host != "example.com" || probes[0].Err != nil {
		t.Fatalf("probes = %#v, want single successful metadata probe", probes)
	}
}

func TestSelectBestEndpointChoosesReachableCandidate(t *testing.T) {
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
	server := config.Server{Port: port, Hosts: []string{"127.0.0.2", "127.0.0.1"}}
	host, probes, err := SelectBestEndpoint(server)
	if err != nil {
		t.Fatalf("SelectBestEndpoint returned error: %v", err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want 127.0.0.1; probes = %#v", host, probes)
	}
	if len(probes) != 2 {
		t.Fatalf("len(probes) = %d, want 2", len(probes))
	}
}
