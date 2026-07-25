package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// serverArgs builds argv for a spawned `ais serve` (daemon start or restart).
// The pprof flag must be carried across so a daemon/restart keeps the
// diagnostic surface the operator asked for, instead of silently dropping it.
func TestServerArgs(t *testing.T) {
	cases := []struct {
		name              string
		configPath, pprof string
		want              []string
	}{
		{"empty", "", "", []string{"serve"}},
		{"config only", "/c.yaml", "", []string{"serve", "-c", "/c.yaml"}},
		{"pprof only", "", "127.0.0.1:6060", []string{"serve", "--pprof", "127.0.0.1:6060"}},
		{"both", "/c.yaml", "127.0.0.1:6060", []string{"serve", "-c", "/c.yaml", "--pprof", "127.0.0.1:6060"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, serverArgs(c.configPath, c.pprof))
		})
	}
}
