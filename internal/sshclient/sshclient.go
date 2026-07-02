package sshclient

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
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
	methods, err := authMethods(server, keyboard)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{User: server.Username, Auth: methods, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 15 * time.Second}
	addr := net.JoinHostPort(server.Host, fmt.Sprint(server.Port))
	log.Printf("connecting to SSH server %s as %s", addr, server.Username)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh authentication/connect failed; check auth credentials, TOTP seed, and local/server time: %w", err)
	}
	log.Printf("SSH connection established for %s", name)
	return client, nil
}

func authMethods(server config.Server, keyboard ssh.AuthMethod) ([]ssh.AuthMethod, error) {
	switch server.Auth.Type {
	case "password_totp":
		return []ssh.AuthMethod{ssh.Password(server.Auth.Password), keyboard}, nil
	case "key_totp":
		signer, err := privateKeySigner(server.Auth.KeyPath, server.Auth.KeyPassphrase)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer), keyboard}, nil
	default:
		return nil, fmt.Errorf("unsupported auth.type %q", server.Auth.Type)
	}
}

func privateKeySigner(path, passphrase string) (ssh.Signer, error) {
	expandedPath, err := expandPath(path)
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", expandedPath, err)
	}
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("parse encrypted private key %s: %w", expandedPath, err)
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w", expandedPath, err)
	}
	return signer, nil
}

func expandPath(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
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
