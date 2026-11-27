package curl

import (
	"regexp"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
)

// shell is how a command was written: its line continuations and quotes.
type shell int

const (
	bash       shell = iota // \ continues a line; '…' literal, "…" with \ escapes
	cmd                     // ^ continues a line and escapes; "…" with \" (Windows)
	powershell              // ` continues a line and escapes; '…' with '', "…" with `
)

var (
	cmdContinuation  = regexp.MustCompile(`\^\r?\n`)
	psContinuation   = regexp.MustCompile("`\r?\n")
	bashContinuation = regexp.MustCompile(`\\\r?\n`)
)

// detect tells the shell a command was written for.
func detect(s string) shell {
	switch {
	case cmdContinuation.MatchString(s) || strings.Contains(s, `^"`):
		return cmd
	case psContinuation.MatchString(s):
		return powershell
	}
	return bash
}

// tokenize splits a command line into its words, as the shell would.
func tokenize(s string) ([]string, error) {
	sh := detect(s)
	switch sh {
	case cmd:
		s = cmdContinuation.ReplaceAllString(s, " ")
		s = uncaret(s)
	case powershell:
		s = psContinuation.ReplaceAllString(s, " ")
	default:
		s = bashContinuation.ReplaceAllString(s, " ")
	}

	var words []string
	var cur strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		case c == '\'' && sh != cmd:
			inWord = true
			j := i + 1
			for ; j < len(s); j++ {
				if s[j] == '\'' {
					if sh == powershell && j+1 < len(s) && s[j+1] == '\'' {
						cur.WriteByte('\'') // '' is a quote in PowerShell
						j++
						continue
					}
					break
				}
				cur.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, apperr.New(CodeInvalid).With("reason", "quote")
			}
			i = j
		case c == '"':
			inWord = true
			j := i + 1
			for ; j < len(s); j++ {
				d := s[j]
				if d == '"' {
					break
				}
				switch {
				case sh == bash && d == '\\' && j+1 < len(s) && strings.IndexByte("\"\\$`\n", s[j+1]) >= 0:
					j++
					d = s[j]
				case sh == cmd && d == '\\' && j+1 < len(s) && (s[j+1] == '"' || s[j+1] == '\\'):
					j++
					d = s[j]
				case sh == powershell && d == '`' && j+1 < len(s):
					j++
					d = s[j]
				}
				cur.WriteByte(d)
			}
			if j >= len(s) {
				return nil, apperr.New(CodeInvalid).With("reason", "quote")
			}
			i = j
		case c == '\\' && sh == bash && i+1 < len(s):
			inWord = true
			i++
			cur.WriteByte(s[i])
		case c == '`' && sh == powershell && i+1 < len(s):
			inWord = true
			i++
			cur.WriteByte(s[i])
		default:
			inWord = true
			cur.WriteByte(c)
		}
	}
	flush()
	return words, nil
}

// uncaret removes cmd's escape character: ^x is x, ^^ is ^.
func uncaret(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '^' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
