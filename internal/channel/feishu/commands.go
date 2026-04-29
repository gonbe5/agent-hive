package feishu

import (
	"strings"
	"unicode"
)

var commandWhitelist = map[string]struct{}{
	"/help":   {},
	"/status": {},
	"/reset":  {},
	"/debug":  {},
	"/mute":   {},
	"/model":  {},
	"/agent":  {},
	"/unmute": {},
	"/audit":  {},
}

type ParsedCommand struct {
	Name string
	Raw  string
	Arg  string
	Args []string
}

func NormalizeCommand(input string) string {
	input = strings.TrimSpace(input)
	input = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, input)
	return input
}

func ParseCommand(input string) (*ParsedCommand, bool) {
	fields := strings.Fields(NormalizeCommand(input))
	if len(fields) == 0 {
		return nil, false
	}
	raw := fields[0]
	if _, ok := commandWhitelist[raw]; !ok {
		return nil, false
	}
	arg := ""
	if len(fields) > 1 {
		arg = fields[1]
	}
	return &ParsedCommand{
		Name: strings.TrimPrefix(raw, "/"),
		Raw:  raw,
		Arg:  arg,
		Args: append([]string(nil), fields[1:]...),
	}, true
}
