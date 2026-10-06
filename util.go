package starlings

import (
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func itoa(n int) string { return strconv.Itoa(n) }

func appendInt(b []byte, n int64) []byte { return strconv.AppendInt(b, n, 10) }

// runtimeOS is reported to Discord in the identify payload, which is only ever
// used for their own statistics.
func runtimeOS() string { return runtime.GOOS }

// urlEscape percent-encodes a query parameter value.
func urlEscape(s string) string { return url.QueryEscape(s) }

// joinSpace joins OAuth2 scopes, which are space separated.
func joinSpace(parts []string) string { return strings.Join(parts, " ") }

// sanitizeUntrustedText makes terminal control sequences and invisible bidi
// overrides visible before remote Discord data reaches a terminal or log
// file. It deliberately preserves ordinary Unicode text.
func sanitizeUntrustedText(s string) string {
	needsSanitizing := !utf8.ValidString(s)
	if !needsSanitizing {
		for _, r := range s {
			if r == '\n' || r == '\r' || r == '\t' || r == '\x1b' || unicode.IsControl(r) || isBidiControl(r) {
				needsSanitizing = true
				break
			}
		}
	}
	if !needsSanitizing {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		case '\x1b':
			out.WriteString(`\x1b`)
		default:
			if unicode.IsControl(r) || isBidiControl(r) {
				if r <= 0xffff {
					fmt.Fprintf(&out, `\u%04x`, r)
				} else {
					fmt.Fprintf(&out, `\U%08x`, r)
				}
			} else {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

func isBidiControl(r rune) bool {
	return r == '\u061c' || r == '\u200e' || r == '\u200f' ||
		(r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}
