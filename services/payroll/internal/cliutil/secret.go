// Package cliutil holds helpers shared by the payroll CLIs (cmd/*).
package cliutil

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// Warn receives the argv warning; tests replace it.
var Warn io.Writer = os.Stderr

// secretValue is a string flag for DSNs and keys. Its default comes from the environment and is never printed:
// String always returns "", which flag.PrintDefaults treats as the zero value and omits (so -h and usage errors
// cannot leak it). Passing it as an argument is allowed but warned about, because argv is visible in the process
// list and shell history.
type secretValue struct {
	p    *string
	name string
	env  string
}

func (s *secretValue) String() string { return "" }

func (s *secretValue) Set(v string) error {
	*s.p = v
	_, _ = fmt.Fprintf(Warn, "peringatan: -%s lewat argumen terlihat di daftar proses dan riwayat shell; pakai env %s\n", s.name, s.env) //nolint:errcheck // best effort
	return nil
}

// SecretString registers flag name on fs and returns the pointer to read after Parse. The default is the first
// non-empty variable among envs (the first one is the canonical name, later ones are legacy fallbacks).
func SecretString(fs *flag.FlagSet, name, usage string, envs ...string) *string {
	var v string
	for _, e := range envs {
		if v = os.Getenv(e); v != "" {
			break
		}
	}
	fs.Var(&secretValue{p: &v, name: name, env: envs[0]}, name, usage+" (env "+envs[0]+"; jangan lewat argumen)")
	return &v
}
