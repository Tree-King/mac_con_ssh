package main

import "testing"

func TestCommonFlagsAcceptsConfigAfterServer(t *testing.T) {
	server, cfgPath, err := commonFlags("run", []string{"my-server", "--config", "/tmp/config.yaml"})
	if err != nil {
		t.Fatalf("commonFlags returned error: %v", err)
	}
	if server != "my-server" {
		t.Fatalf("server = %q, want %q", server, "my-server")
	}
	if cfgPath != "/tmp/config.yaml" {
		t.Fatalf("cfgPath = %q, want %q", cfgPath, "/tmp/config.yaml")
	}
}

func TestCommonFlagsAcceptsConfigBeforeServer(t *testing.T) {
	server, cfgPath, err := commonFlags("run", []string{"--config", "/tmp/config.yaml", "my-server"})
	if err != nil {
		t.Fatalf("commonFlags returned error: %v", err)
	}
	if server != "my-server" {
		t.Fatalf("server = %q, want %q", server, "my-server")
	}
	if cfgPath != "/tmp/config.yaml" {
		t.Fatalf("cfgPath = %q, want %q", cfgPath, "/tmp/config.yaml")
	}
}

func TestCommonFlagsAcceptsServerFlagAfterConfig(t *testing.T) {
	server, cfgPath, err := commonFlags("run", []string{"--config", "/tmp/config.yaml", "--server", "my-server"})
	if err != nil {
		t.Fatalf("commonFlags returned error: %v", err)
	}
	if server != "my-server" {
		t.Fatalf("server = %q, want %q", server, "my-server")
	}
	if cfgPath != "/tmp/config.yaml" {
		t.Fatalf("cfgPath = %q, want %q", cfgPath, "/tmp/config.yaml")
	}
}

func TestCommonFlagsAcceptsConfigEqualsAfterServer(t *testing.T) {
	server, cfgPath, err := commonFlags("run", []string{"my-server", "--config=/tmp/config.yaml"})
	if err != nil {
		t.Fatalf("commonFlags returned error: %v", err)
	}
	if server != "my-server" {
		t.Fatalf("server = %q, want %q", server, "my-server")
	}
	if cfgPath != "/tmp/config.yaml" {
		t.Fatalf("cfgPath = %q, want %q", cfgPath, "/tmp/config.yaml")
	}
}

func TestRunCommandUsesConfigAfterServer(t *testing.T) {
	cfgPath := t.TempDir() + "/missing-config.yaml"
	err := runCommand([]string{"my-server", "--config", cfgPath})
	if err == nil {
		t.Fatal("runCommand returned nil error, want missing config error")
	}
	if got := err.Error(); got != "open "+cfgPath+": no such file or directory" {
		t.Fatalf("error = %q, want missing custom config path", got)
	}
}

func TestCommonRunFlagsAcceptsMultipleServers(t *testing.T) {
	flags, err := commonRunFlags([]string{"ssh-service", "tcp-service", "--config", "/tmp/config.yaml"})
	if err != nil {
		t.Fatalf("commonRunFlags returned error: %v", err)
	}
	wantServers := []string{"ssh-service", "tcp-service"}
	if len(flags.servers) != len(wantServers) {
		t.Fatalf("servers = %#v, want %#v", flags.servers, wantServers)
	}
	for i := range wantServers {
		if flags.servers[i] != wantServers[i] {
			t.Fatalf("servers = %#v, want %#v", flags.servers, wantServers)
		}
	}
	if flags.cfgPath != "/tmp/config.yaml" {
		t.Fatalf("cfgPath = %q, want %q", flags.cfgPath, "/tmp/config.yaml")
	}
}

func TestCommonRunFlagsAcceptsRepeatedServerFlags(t *testing.T) {
	flags, err := commonRunFlags([]string{"--config", "/tmp/config.yaml", "--server", "ssh-service", "--server=tcp-service"})
	if err != nil {
		t.Fatalf("commonRunFlags returned error: %v", err)
	}
	wantServers := []string{"ssh-service", "tcp-service"}
	if len(flags.servers) != len(wantServers) {
		t.Fatalf("servers = %#v, want %#v", flags.servers, wantServers)
	}
	for i := range wantServers {
		if flags.servers[i] != wantServers[i] {
			t.Fatalf("servers = %#v, want %#v", flags.servers, wantServers)
		}
	}
	if flags.cfgPath != "/tmp/config.yaml" {
		t.Fatalf("cfgPath = %q, want %q", flags.cfgPath, "/tmp/config.yaml")
	}
}
