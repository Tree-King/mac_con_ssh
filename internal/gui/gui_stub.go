//go:build !gui

package gui

import "fmt"

// Run is available in binaries built with the gui build tag.
func Run(configPath string) error {
	return fmt.Errorf("GUI support is not included in this binary; rebuild with: go build -tags gui ./cmd/ssh-tunnel")
}
