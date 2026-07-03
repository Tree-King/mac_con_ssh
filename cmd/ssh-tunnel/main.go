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
	"sync"
	"syscall"

	"ssh-tunnel/internal/config"
	"ssh-tunnel/internal/runner"
	"ssh-tunnel/internal/sshclient"
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

type runFlags struct {
	servers []string
	cfgPath string
}

func commonRunFlags(args []string) (runFlags, error) {
	flags := runFlags{cfgPath: config.DefaultPath()}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			flags.servers = append(flags.servers, args[i+1:]...)
			i = len(args)
		case arg == "--config" || arg == "-config":
			if i+1 >= len(args) {
				return runFlags{}, fmt.Errorf("flag needs an argument: %s", arg)
			}
			flags.cfgPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			flags.cfgPath = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-config="):
			flags.cfgPath = strings.TrimPrefix(arg, "-config=")
		case arg == "--server" || arg == "-server":
			if i+1 >= len(args) {
				return runFlags{}, fmt.Errorf("flag needs an argument: %s", arg)
			}
			flags.servers = append(flags.servers, args[i+1])
			i++
		case strings.HasPrefix(arg, "--server="):
			flags.servers = append(flags.servers, strings.TrimPrefix(arg, "--server="))
		case strings.HasPrefix(arg, "-server="):
			flags.servers = append(flags.servers, strings.TrimPrefix(arg, "-server="))
		case strings.HasPrefix(arg, "-") && arg != "-":
			return runFlags{}, fmt.Errorf("flag provided but not defined: %s", strings.TrimLeft(arg, "-"))
		default:
			flags.servers = append(flags.servers, arg)
		}
	}
	if len(flags.servers) == 0 {
		return runFlags{}, errors.New("missing server name")
	}
	return flags, nil
}

func runCommand(args []string) error {
	flags, err := commonRunFlags(args)
	if err != nil {
		return err
	}
	log.Printf("reading configuration from %s", flags.cfgPath)
	cfg, warnings, err := config.Load(flags.cfgPath)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		log.Printf("warning: %s", warning)
	}
	servers := make(map[string]config.Server, len(flags.servers))
	for _, serverName := range flags.servers {
		server, err := cfg.Server(serverName)
		if err != nil {
			return err
		}
		servers[serverName] = server
		logServerMode(serverName, server)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runServers(ctx, servers)
}

func logServerMode(serverName string, server config.Server) {
	if server.Direct {
		log.Printf("server %s uses direct TCP forwarding", serverName)
		return
	}
	log.Printf("server %s uses auth.type %s", serverName, server.Auth.Type)
}

func runServers(ctx context.Context, servers map[string]config.Server) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, len(servers))
	var wg sync.WaitGroup
	for serverName, server := range servers {
		serverName, server := serverName, server
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runner.Run(ctx, serverName, server); err != nil && ctx.Err() == nil {
				errc <- fmt.Errorf("server %s: %w", serverName, err)
				cancel()
			}
		}()
	}
	go func() {
		wg.Wait()
		close(errc)
	}()
	for err := range errc {
		if err != nil {
			return err
		}
	}
	return nil
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
	if server.Direct {
		log.Printf("server %s uses direct TCP forwarding", serverName)
	} else {
		log.Printf("server %s uses auth.type %s", serverName, server.Auth.Type)
	}
	if err := server.Validate(); err != nil {
		return err
	}
	if !server.Direct {
		if _, err := totp.Generate(server.Auth.TOTPSeed); err != nil {
			return fmt.Errorf("totp seed is invalid: %w", err)
		}
	}
	for _, fwd := range server.Forwards {
		addr := net.JoinHostPort(fwd.LocalHost, fmt.Sprint(fwd.LocalPort))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("local forward %s cannot listen on %s: %w", fwd.Name, addr, err)
		}
		_ = ln.Close()
	}
	if !server.Direct {
		host, probes, err := sshclient.SelectBestEndpoint(server)
		sshclient.LogEndpointSelection(serverName, host, probes)
		if err != nil {
			return fmt.Errorf("ssh endpoint is not reachable: %w", err)
		}
	}
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
  ssh-tunnel run SERVER [SERVER ...] [--config PATH]
  ssh-tunnel run --server SERVER [--server SERVER ...] [--config PATH]
  ssh-tunnel check SERVER [--config PATH]
  ssh-tunnel totp SERVER [--config PATH]
  ssh-tunnel version`))
}
