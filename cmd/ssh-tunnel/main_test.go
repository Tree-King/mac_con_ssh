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
