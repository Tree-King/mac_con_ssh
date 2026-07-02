package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"ssh-tunnel/internal/config"
)

type Need int

const (
	NeedPassword Need = 1 << iota
	NeedTOTPSeed
)

func Resolve(server config.Server, need Need) (config.Server, error) {
	if need&NeedPassword != 0 && server.Auth.Password == "" {
		value, err := readSecret("SSH password")
		if err != nil {
			return server, err
		}
		server.Auth.Password = value
	}
	if need&NeedTOTPSeed != 0 && server.Auth.TOTPSeed == "" {
		value, err := readSecret("TOTP seed")
		if err != nil {
			return server, err
		}
		server.Auth.TOTPSeed = value
	}
	return server, nil
}

func readSecret(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("%s is not configured and secure interactive input requires a terminal", label)
	}
	fmt.Fprintf(os.Stderr, "Enter %s: ", label)
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(bytes))
	if value == "" {
		return "", errors.New(label + " cannot be empty")
	}
	return value, nil
}
