package cli

import (
	"strings"
)

// Compound shell statements for automation exercises:
//
//	for VAR in WORDS...; do ...; done
//	if COMMAND; then ...; [else ...;] fi
//
// Both work on one line or across lines and can be nested. WORDS undergo
// variable expansion and command substitution, and unquoted results are
// split on whitespace, as in bash.

// statements splits text into statements at top-level ';' and newlines and
// separates the keywords do/then/else from the command that follows them.
func statements(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		parts, _ := splitTop(line, []string{";"})
		for _, p := range parts {
			p = strings.TrimSpace(p)
			for {
				moved := false
				for _, kw := range []string{"do", "then", "else"} {
					if p == kw {
						break
					}
					if strings.HasPrefix(p, kw+" ") {
						out = append(out, kw)
						p = strings.TrimSpace(p[len(kw):])
						moved = true
					}
				}
				if !moved {
					break
				}
			}
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func blockDelta(stmt string) int {
	switch firstWord(stmt) {
	case "for", "if", "while":
		return 1
	case "done", "fi":
		return -1
	}
	return 0
}

// OpenBlocks reports how many for/if blocks are still open at the end of text
// (used by script runners to keep compound statements together).
func OpenBlocks(text string) int { return openBlocks(text) }

// openBlocks returns the nesting depth after the statements of a text.
func openBlocks(text string) int {
	d := 0
	for _, st := range statements(text) {
		d += blockDelta(st)
	}
	return d
}

// runStatements executes a list of statements, handling nested blocks.
func (s *Session) runStatements(stmts []string) Result {
	var out strings.Builder
	last := Result{}
	for i := 0; i < len(stmts); i++ {
		st := stmts[i]
		if blockDelta(st) == 1 {
			depth, j := 0, i
			for ; j < len(stmts); j++ {
				depth += blockDelta(stmts[j])
				if depth == 0 {
					break
				}
			}
			if j >= len(stmts) {
				return Result{Output: out.String() + "bash: syntax error: unexpected end of file (missing done/fi)\n", Exit: 2}
			}
			last = s.runBlock(stmts[i : j+1])
			out.WriteString(last.Output)
			i = j
			continue
		}
		last = s.execLine(st, "")
		out.WriteString(last.Output)
	}
	return Result{Output: out.String(), Exit: last.Exit}
}

// runBlock executes one for/if block (first statement opens, last closes).
func (s *Session) runBlock(stmts []string) Result {
	head := stmts[0]
	body := stmts[1 : len(stmts)-1]
	switch firstWord(head) {
	case "for":
		f := strings.Fields(head)
		if len(f) < 3 || f[2] != "in" {
			return Result{Output: "bash: syntax error near `for': expected `for NAME in WORDS; do ...; done'\n", Exit: 2}
		}
		name := f[1]
		listSrc := strings.TrimSpace(head[strings.Index(head, " in ")+4:])
		toks, err := s.tokenize(listSrc)
		if err != nil {
			return Result{Output: err.Error() + "\n", Exit: 2}
		}
		var words []string
		for _, t := range toks {
			words = append(words, strings.Fields(t)...)
		}
		if len(body) == 0 || body[0] != "do" {
			return Result{Output: "bash: syntax error near `for': missing `do'\n", Exit: 2}
		}
		var out strings.Builder
		last := Result{}
		for _, w := range words {
			s.Env[name] = w
			last = s.runStatements(body[1:])
			out.WriteString(last.Output)
		}
		return Result{Output: out.String(), Exit: last.Exit}
	case "if":
		cond := strings.TrimSpace(strings.TrimPrefix(head, "if"))
		var thenPart, elsePart []string
		cur := &thenPart
		depth := 0
		seenThen := false
		for _, st := range body {
			if depth == 0 && st == "then" && !seenThen {
				seenThen = true
				continue
			}
			if depth == 0 && st == "else" {
				cur = &elsePart
				continue
			}
			depth += blockDelta(st)
			*cur = append(*cur, st)
		}
		if !seenThen {
			return Result{Output: "bash: syntax error near `if': missing `then'\n", Exit: 2}
		}
		c := s.execLine(cond, "")
		out := c.Output
		var r Result
		if c.Exit == 0 {
			r = s.runStatements(thenPart)
		} else {
			r = s.runStatements(elsePart)
		}
		return Result{Output: out + r.Output, Exit: r.Exit}
	}
	return Result{Output: "bash: " + firstWord(head) + ": compound command not supported\n", Exit: 2}
}
