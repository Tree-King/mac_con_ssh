package main

import (
	"context"
	"errors"
	"flag"
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
		fmt.Printf("ssh-tunnel %s\n", version)
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func commonFlags(name string, args []string) (string, string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfgPath := fs.String("config", config.DefaultPath(), "configuration file path")
	serverFlag := fs.String("server", "", "server name")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	server := *serverFlag
	if server == "" && fs.NArg() > 0 {
		server = fs.Arg(0)
	}
	if server == "" {
		return "", "", errors.New("missing server name")
	}
	return server, *cfgPath, nil
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
