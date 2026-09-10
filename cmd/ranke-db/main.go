// package: main / cmd
// type:    entrypoint
// job:     the ranke-db binary — a cobra CLI handing a config to the config package
// limits:  CLI wiring only; decrypt/parse/resolve/assemble live in config (-> config)
//
// One file per command; what sits here is the tree they hang off and the plumbing they
// share — opening the launch artifact, and sourcing the age key that decrypts it.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/rankegraph/ranke-db/config"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ranke-db:", err)
		os.Exit(1)
	}
}

// rootCmd builds the ranke-db command tree. Silence* keeps cobra from dumping
// usage and a second error line on a runtime failure — main prints the error.
func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ranke-db",
		Short:         "Serve a Ranke-Graph from a launch artifact",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(runCmd(), verifyCmd(), foundCmd(), versionCmd())
	return root
}

// openConfig opens the launch artifact as a reader: a file (closed by cleanup)
// or stdin. Reading both the config and the age key from stdin is refused.
func openConfig(path, ageKey string) (io.Reader, func(), error) {
	if path == "-" {
		if ageKey == "stdin" {
			return nil, nil, errors.New("cannot read both the config and the age key from stdin; use --age-key prompt|env:VAR|file:path")
		}
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open config: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

// passphraseFrom builds a config.PassphraseSource from a spec: prompt, stdin,
// env:VAR, file:path, or empty for none. A literal passphrase is deliberately
// unsupported — never pass the key as a command-line argument.
func passphraseFrom(spec string, in io.Reader) (config.PassphraseSource, error) {
	switch {
	case spec == "":
		return nil, nil
	case spec == "prompt":
		return promptPassphrase, nil
	case spec == "stdin":
		return func() (string, error) {
			b, err := io.ReadAll(in)
			if err != nil {
				return "", fmt.Errorf("read age passphrase from stdin: %w", err)
			}
			return strings.TrimRight(string(b), "\r\n"), nil
		}, nil
	case strings.HasPrefix(spec, "env:"):
		name := strings.TrimPrefix(spec, "env:")
		return func() (string, error) {
			v, ok := os.LookupEnv(name)
			if !ok {
				return "", fmt.Errorf("age key env(%s) is not set", name)
			}
			return v, nil
		}, nil
	case strings.HasPrefix(spec, "file:"):
		path := strings.TrimPrefix(spec, "file:")
		return func() (string, error) {
			b, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("read age key file: %w", err)
			}
			return strings.TrimRight(string(b), "\r\n"), nil
		}, nil
	default:
		return nil, fmt.Errorf("unknown age key source %q; use prompt|stdin|env:VAR|file:path", spec)
	}
}

// promptPassphrase reads from the controlling terminal without echo, opening /dev/tty
// directly so it works when stdin is the config pipe.
func promptPassphrase() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("open terminal for passphrase prompt: %w", err)
	}
	defer func() { _ = tty.Close() }()
	fmt.Fprint(tty, "age passphrase: ")
	b, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	return string(b), nil
}
