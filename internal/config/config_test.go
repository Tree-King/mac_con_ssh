package config

import "testing"

func validServer(auth Auth) Server {
	return Server{
		Host:     "example.com",
		Port:     22,
		Username: "root",
		Auth:     auth,
		Forwards: []Forward{{
			Name:       "db",
			LocalHost:  "127.0.0.1",
			LocalPort:  3307,
			RemoteHost: "127.0.0.1",
			RemotePort: 3306,
		}},
	}
}

func TestValidateAcceptsPasswordTOTP(t *testing.T) {
	srv := validServer(Auth{Type: "password_totp", Password: "secret", TOTPSeed: "JBSWY3DPEHPK3PXP"})
	if err := srv.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateAcceptsMultipleHosts(t *testing.T) {
	srv := validServer(Auth{Type: "password_totp", Password: "secret", TOTPSeed: "JBSWY3DPEHPK3PXP"})
	srv.Host = ""
	srv.Hosts = []string{"203.0.113.10", "203.0.113.11"}
	if err := srv.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestHostCandidatesKeepsHostFirstAndDeduplicates(t *testing.T) {
	srv := validServer(Auth{Type: "password_totp", Password: "secret", TOTPSeed: "JBSWY3DPEHPK3PXP"})
	srv.Host = "203.0.113.10"
	srv.Hosts = []string{"203.0.113.11", "203.0.113.10"}
	got := srv.HostCandidates()
	want := []string{"203.0.113.10", "203.0.113.11"}
	if len(got) != len(want) {
		t.Fatalf("HostCandidates length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("HostCandidates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestForwardDirectCandidatesSupportsTargets(t *testing.T) {
	fwd := Forward{
		DirectTargets: []ForwardTarget{
			{Host: "db-a.example.com", Port: 3306},
			{Host: "db-b.example.com", Port: 3306},
			{Host: "db-a.example.com", Port: 3306},
		},
	}
	got := fwd.DirectCandidates()
	want := []ForwardTarget{
		{Host: "db-a.example.com", Port: 3306},
		{Host: "db-b.example.com", Port: 3306},
	}
	if len(got) != len(want) {
		t.Fatalf("DirectCandidates length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DirectCandidates[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestValidateAcceptsDirectForwardWithoutSSHAuth(t *testing.T) {
	srv := Server{
		Direct: true,
		Forwards: []Forward{{
			Name:      "db",
			LocalHost: "127.0.0.1",
			LocalPort: 3307,
			DirectTargets: []ForwardTarget{
				{Host: "db-a.example.com", Port: 3306},
				{Host: "db-b.example.com", Port: 3306},
			},
		}},
	}
	if err := srv.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateAcceptsKeyTOTP(t *testing.T) {
	srv := validServer(Auth{Type: "key_totp", KeyPath: "~/.ssh/id_ed25519", TOTPSeed: "JBSWY3DPEHPK3PXP"})
	if err := srv.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateKeyTOTPRequiresKeyPath(t *testing.T) {
	srv := validServer(Auth{Type: "key_totp", TOTPSeed: "JBSWY3DPEHPK3PXP"})
	if err := srv.Validate(); err == nil || err.Error() != "auth.key_path is required" {
		t.Fatalf("Validate error = %v, want auth.key_path is required", err)
	}
}

func TestValidateUnsupportedAuthTypeListsKeyTOTP(t *testing.T) {
	srv := validServer(Auth{Type: "key-totp", KeyPath: "~/.ssh/id_ed25519", TOTPSeed: "JBSWY3DPEHPK3PXP"})
	err := srv.Validate()
	if err == nil {
		t.Fatal("Validate returned nil error, want unsupported auth.type")
	}
	want := "auth.type must be password_totp or key_totp"
	if err.Error() != want {
		t.Fatalf("Validate error = %q, want %q", err.Error(), want)
	}
}
