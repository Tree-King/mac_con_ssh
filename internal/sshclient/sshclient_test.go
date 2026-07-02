package sshclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	otp "github.com/pquerna/otp/totp"
	"golang.org/x/crypto/ssh"

	"ssh-tunnel/internal/config"
)

const testTOTPSeed = "JBSWY3DPEHPK3PXP"

func TestConnectKeyTOTPAgainstSimulatedSSHServer(t *testing.T) {
	_, clientPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	clientSigner, err := ssh.NewSignerFromKey(clientPrivateKey)
	if err != nil {
		t.Fatalf("NewSignerFromKey returned error: %v", err)
	}
	keyPath := writeOpenSSHPrivateKey(t, clientPrivateKey)

	var sawClientKey atomic.Bool
	var sawTOTP atomic.Bool
	addr, stop := startSimulatedTOTPServer(t, func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() != "tunnel" {
			return nil, errors.New("unexpected user")
		}
		if string(key.Marshal()) == string(clientSigner.PublicKey().Marshal()) {
			sawClientKey.Store(true)
		}
		return nil, errors.New("continue with keyboard-interactive TOTP")
	}, func(conn ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		if !sawClientKey.Load() {
			return nil, errors.New("private key was not offered before TOTP")
		}
		answers, err := challenge("TOTP required", "Enter authenticator code", []string{"Verification code: "}, []bool{false})
		if err != nil {
			return nil, err
		}
		if len(answers) != 1 {
			return nil, errors.New("expected one TOTP answer")
		}
		want, err := otp.GenerateCode(testTOTPSeed, time.Now())
		if err != nil {
			return nil, err
		}
		if answers[0] != want {
			return nil, errors.New("unexpected TOTP code")
		}
		sawTOTP.Store(true)
		return nil, nil
	})
	defer stop()

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort returned error: %v", err)
	}
	srv := config.Server{
		Host:     host,
		Port:     mustAtoi(t, port),
		Username: "tunnel",
		Auth: config.Auth{
			Type:     "key_totp",
			KeyPath:  keyPath,
			TOTPSeed: testTOTPSeed,
		},
	}
	client, err := Connect("simulated-key-totp", srv)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	_ = client.Close()
	if !sawClientKey.Load() {
		t.Fatal("simulated server did not receive the configured private key")
	}
	if !sawTOTP.Load() {
		t.Fatal("simulated server did not receive a valid TOTP response")
	}
}

func writeOpenSSHPrivateKey(t *testing.T, key ed25519.PrivateKey) string {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(key, "test-key")
	if err != nil {
		t.Fatalf("MarshalPrivateKey returned error: %v", err)
	}
	path := t.TempDir() + "/id_ed25519"
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return path
}

func startSimulatedTOTPServer(t *testing.T, publicKeyCallback func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error), keyboardCallback func(ssh.ConnMetadata, ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error)) (string, func()) {
	t.Helper()
	_, hostPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivateKey)
	if err != nil {
		t.Fatalf("NewSignerFromKey returned error: %v", err)
	}
	serverConfig := &ssh.ServerConfig{
		MaxAuthTries:                -1,
		PublicKeyCallback:           publicKeyCallback,
		KeyboardInteractiveCallback: keyboardCallback,
	}
	serverConfig.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(reqs)
		for ch := range chans {
			_ = ch.Reject(ssh.Prohibited, "no sessions in simulated server")
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("simulated SSH server did not stop")
		}
	}
}

func mustAtoi(t *testing.T, value string) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(value, "%d", &port); err != nil {
		t.Fatalf("parse port %q: %v", value, err)
	}
	return port
}
