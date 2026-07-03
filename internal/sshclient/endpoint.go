package sshclient

import (
	"fmt"
	"log"
	"net"
	"time"

	"ssh-tunnel/internal/config"
)

const EndpointProbeTimeout = 3 * time.Second

type EndpointProbe struct {
	Host    string
	Address string
	Latency time.Duration
	Err     error
}

func SelectBestEndpoint(server config.Server) (string, []EndpointProbe, error) {
	hosts := server.HostCandidates()
	if len(hosts) == 0 {
		return "", nil, fmt.Errorf("no SSH host candidates configured")
	}
	if len(hosts) == 1 {
		return hosts[0], []EndpointProbe{{Host: hosts[0], Address: net.JoinHostPort(hosts[0], fmt.Sprint(server.Port))}}, nil
	}

	probes := make([]EndpointProbe, 0, len(hosts))
	bestHost := ""
	var bestLatency time.Duration
	for _, host := range hosts {
		addr := net.JoinHostPort(host, fmt.Sprint(server.Port))
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, EndpointProbeTimeout)
		latency := time.Since(start)
		if err == nil {
			_ = conn.Close()
		}
		probes = append(probes, EndpointProbe{Host: host, Address: addr, Latency: latency, Err: err})
		if err == nil && (bestHost == "" || latency < bestLatency) {
			bestHost = host
			bestLatency = latency
		}
	}
	if bestHost == "" {
		return "", probes, fmt.Errorf("no SSH host candidates are reachable")
	}
	return bestHost, probes, nil
}

func LogEndpointSelection(serverName string, selected string, probes []EndpointProbe) {
	if len(probes) <= 1 {
		return
	}
	for _, probe := range probes {
		if probe.Err != nil {
			log.Printf("server %s candidate %s unreachable after %s: %v", serverName, probe.Address, probe.Latency.Round(time.Millisecond), probe.Err)
			continue
		}
		log.Printf("server %s candidate %s reachable in %s", serverName, probe.Address, probe.Latency.Round(time.Millisecond))
	}
	log.Printf("server %s selected SSH host %s", serverName, selected)
}
