package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ssh-tunnel/internal/config"
	"ssh-tunnel/internal/runner"
	"ssh-tunnel/internal/totp"
)

var version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := run(os.Args[1:]); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing command")
	}
	switch args[0] {
	case "run":
		return runCommand(args[1:])
	case "check":
		return checkCommand(args[1:])
	case "totp":
		return totpCommand(args[1:])
	case "version":
		fmt.Printf("ssh-tunnel %s (auth: %s)\n", version, config.SupportedAuthTypesText())
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func commonFlags(_ string, args []string) (string, string, error) {
	cfgPath := config.DefaultPath()
	server := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			if server == "" && i+1 < len(args) {
				server = args[i+1]
			}
			i = len(args)
		case arg == "--config" || arg == "-config":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("flag needs an argument: %s", arg)
			}
			cfgPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			cfgPath = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-config="):
			cfgPath = strings.TrimPrefix(arg, "-config=")
		case arg == "--server" || arg == "-server":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("flag needs an argument: %s", arg)
			}
			server = args[i+1]
			i++
		case strings.HasPrefix(arg, "--server="):
			server = strings.TrimPrefix(arg, "--server=")
		case strings.HasPrefix(arg, "-server="):
			server = strings.TrimPrefix(arg, "-server=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return "", "", fmt.Errorf("flag provided but not defined: %s", strings.TrimLeft(arg, "-"))
		case server == "":
			server = arg
		}
	}
	if server == "" {
		return "", "", errors.New("missing server name")
	}
	return server, cfgPath, nil
}

func runCommand(args []string) error {
	serverName, cfgPath, err := commonFlags("run", args)
	if err != nil {
		return err
	}
	log.Printf("reading configuration from %s", cfgPath)
	cfg, warnings, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		log.Printf("warning: %s", warning)
	}
	server, err := cfg.Server(serverName)
	if err != nil {
		return err
	}
	log.Printf("server %s uses auth.type %s", serverName, server.Auth.Type)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runner.Run(ctx, serverName, server)
}

func checkCommand(args []string) error {
	serverName, cfgPath, err := commonFlags("check", args)
	if err != nil {
		return err
	}
	log.Printf("checking configuration from %s", cfgPath)
	cfg, warnings, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		log.Printf("warning: %s", warning)
	}
	server, err := cfg.Server(serverName)
	if err != nil {
		return err
	}
	log.Printf("server %s uses auth.type %s", serverName, server.Auth.Type)
	if err := server.Validate(); err != nil {
		return err
	}
	if _, err := totp.Generate(server.Auth.TOTPSeed); err != nil {
		return fmt.Errorf("totp seed is invalid: %w", err)
	}
	for _, fwd := range server.Forwards {
		addr := net.JoinHostPort(fwd.LocalHost, fmt.Sprint(fwd.LocalPort))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("local forward %s cannot listen on %s: %w", fwd.Name, addr, err)
		}
		_ = ln.Close()
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(server.Host, fmt.Sprint(server.Port)), 5*time.Second)
	if err != nil {
		return fmt.Errorf("ssh endpoint is not reachable: %w", err)
	}
	_ = conn.Close()
	log.Printf("configuration check passed for %s", serverName)
	return nil
}

func totpCommand(args []string) error {
	serverName, cfgPath, err := commonFlags("totp", args)
	if err != nil {
		return err
	}
	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	server, err := cfg.Server(serverName)
	if err != nil {
		return err
	}
	code, err := totp.Generate(server.Auth.TOTPSeed)
	if err != nil {
		return err
	}
	fmt.Println(code)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, strings.TrimSpace(`Usage:
  ssh-tunnel run SERVER [--config PATH]
  ssh-tunnel run --server SERVER [--config PATH]
  ssh-tunnel check SERVER [--config PATH]
  ssh-tunnel totp SERVER [--config PATH]
  ssh-tunnel version`))
}
