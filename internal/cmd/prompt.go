package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// prompter reads answers from the user during interactive onboarding. It masks secret
// input when stdin is a real terminal, and falls back to a plain line read otherwise
// (piped input, tests), so nothing hangs waiting for a tty that isn't there.
type prompter struct {
	in  *bufio.Reader
	out interface{ Write([]byte) (int, error) }
	fd  int
	tty bool
}

func (a *App) prompter() *prompter {
	p := &prompter{in: bufio.NewReader(a.in), out: a.out, fd: -1}
	if f, ok := a.in.(*os.File); ok {
		p.fd = int(f.Fd())
		p.tty = term.IsTerminal(p.fd)
	}
	return p
}

// ask prompts for a value. If def is non-empty it is offered as the default and
// returned when the user just hits enter.
func (p *prompter) ask(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return def, nil
	}
	return answer, nil
}

// askSecret prompts for a value without echoing it, when possible.
func (p *prompter) askSecret(label string) (string, error) {
	fmt.Fprintf(p.out, "%s: ", label)
	if !p.tty {
		line, err := p.in.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	b, err := term.ReadPassword(p.fd)
	fmt.Fprintln(p.out)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
