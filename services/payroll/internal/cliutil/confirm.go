package cliutil

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNotConfirmed is returned when the operator did not confirm an irreversible action.
var ErrNotConfirmed = errors.New("tidak dikonfirmasi")

// Confirm guards an irreversible action (real e-mail). yes (-yes) skips the prompt for scripted runs. Otherwise the
// operator must type the expected token (e.g. the batch id) on in; a non-interactive stdin yields EOF and refuses,
// so a pipeline or cron job can never send by accident.
func Confirm(in io.Reader, out io.Writer, yes bool, summary, token string) error {
	if yes {
		return nil
	}
	_, _ = fmt.Fprintf(out, "%s\nKetik %q untuk melanjutkan (atau pakai -yes): ", summary, token) //nolint:errcheck // prompt
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("%w: stdin tidak interaktif, pakai -yes", ErrNotConfirmed)
	}
	if strings.TrimSpace(line) != token {
		return ErrNotConfirmed
	}
	return nil
}
