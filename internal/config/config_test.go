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
