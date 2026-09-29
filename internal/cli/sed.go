package cli

import (
	"regexp"
	"strings"
)

// sed supports the substitution command, which covers what lab scripts need:
//
//	sed [-i] [-e] 's<d>REGEX<d>REPLACEMENT<d>[g]' [FILE...]
func (s *Session) sed(args []string, stdin string) (string, error) {
	inPlace := false
	var script string
	var files []string
	for _, a := range args[1:] {
		switch {
		case a == "-i" || a == "--in-place":
			inPlace = true
		case a == "-e" || a == "-E" || a == "-r":
		case script == "":
			script = a
		default:
			files = append(files, a)
		}
	}
	if len(script) < 4 || script[0] != 's' {
		return "", fail(1, "sed: only the s/// command is supported by the simulator")
	}
	d := string(script[1])
	parts := strings.Split(script[2:], d)
	if len(parts) < 3 {
		return "", fail(1, "sed: -e expression #1: unterminated `s' command")
	}
	re, err := regexp.Compile(parts[0])
	if err != nil {
		return "", fail(1, "sed: -e expression #1: %v", err)
	}
	repl := strings.ReplaceAll(parts[1], "&", "${0}")
	repl = regexp.MustCompile(`\\([0-9])`).ReplaceAllString(repl, "$${$1}")
	global := strings.Contains(parts[2], "g")
	apply := func(text string) string {
		lines := strings.Split(text, "\n")
		for i, l := range lines {
			if global {
				lines[i] = re.ReplaceAllString(l, repl)
			} else if loc := re.FindStringIndex(l); loc != nil {
				lines[i] = l[:loc[0]] + re.ReplaceAllString(l[loc[0]:loc[1]], repl) + l[loc[1]:]
			}
		}
		return strings.Join(lines, "\n")
	}
	if len(files) == 0 {
		return apply(stdin), nil
	}
	var out strings.Builder
	for _, f := range files {
		content, ok := s.Files[s.path(f)]
		if !ok {
			return "", fail(2, "sed: can't read %s: No such file or directory", f)
		}
		if inPlace {
			s.Files[s.path(f)] = apply(content)
		} else {
			out.WriteString(apply(content))
		}
	}
	return out.String(), nil
}
