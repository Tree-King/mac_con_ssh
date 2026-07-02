package sshclient

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel/internal/config"
	otpgenerator "ssh-tunnel/internal/totp"
)

func Connect(name string, server config.Server) (*ssh.Client, error) {
	keyboard := ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i, q := range questions {
			lower := strings.ToLower(q + " " + instruction)
			if strings.Contains(lower, "verification") || strings.Contains(lower, "authenticator") || strings.Contains(lower, "totp") || strings.Contains(lower, "code") || strings.Contains(lower, "otp") {
				code, err := otpgenerator.Generate(server.Auth.TOTPSeed)
				if err != nil {
					return nil, fmt.Errorf("generate totp code: %w", err)
				}
				answers[i] = code
				continue
			}
			answers[i] = server.Auth.Password
		}
		return answers, nil
	})
	cfg := &ssh.ClientConfig{User: server.Username, Auth: []ssh.AuthMethod{ssh.Password(server.Auth.Password), keyboard}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 15 * time.Second}
	addr := net.JoinHostPort(server.Host, fmt.Sprint(server.Port))
	log.Printf("connecting to SSH server %s as %s", addr, server.Username)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh authentication/connect failed; check password, TOTP seed, and local/server time: %w", err)
	}
	log.Printf("SSH connection established for %s", name)
	return client, nil
}

func StartKeepalive(done <-chan struct{}, client *ssh.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				log.Printf("keepalive failed: %v", err)
				_ = client.Close()
				return
			}
		}
	}
}
