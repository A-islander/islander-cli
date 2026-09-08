package tui

import (
	"regexp"
	"strings"
)

const (
	agentCommandColor = "#a3be8c"
	agentKeywordColor = "#b6a1d7"
	agentPathColor    = "#8bb7cc"
	agentStringColor  = "#c9b27a"
	agentNumberColor  = "#ce9178"
)

// Small display-only lexer for the fixed shell/Python/JSON transcript samples.
// Match original text before adding ANSI, then wrap with the ANSI-aware renderer.
var agentSyntaxTokens = regexp.MustCompile(`"(?:\\.|[^"\\])*"|'[^']*'|[A-Za-z_][A-Za-z0-9_]*=[^\s;]+|(?:\.{1,2}/|~/|/|[A-Za-z0-9_-]+/)[A-Za-z0-9_./*-]*|--?[A-Za-z][A-Za-z0-9-]*|\b(?:Ran|env|go|vet|build|set|cp|cat|python3|rg|git|diff|from|import|package|func|return|true|false|null|[0-9]+)\b|[│└•]`)

func agentHighlightToolLine(line string) string {
	var b strings.Builder
	offset := 0
	for _, span := range agentSyntaxTokens.FindAllStringIndex(line, -1) {
		b.WriteString(ink(line[offset:span[0]], muted))
		token := line[span[0]:span[1]]
		shade := muted
		bold := false
		switch {
		case token == "Ran":
			shade = foam
			bold = true
		case strings.HasPrefix(token, "\"") || strings.HasPrefix(token, "'"):
			shade = agentStringColor
		case strings.HasPrefix(token, "-") || strings.Contains(token, "="):
			shade = agentKeywordColor
		case strings.Contains(token, "/"):
			shade = agentPathColor
		case token == "from" || token == "import" || token == "package" || token == "func" || token == "return":
			shade = agentKeywordColor
		case token == "true" || token == "false" || token == "null":
			shade = agentNumberColor
		case token[0] >= '0' && token[0] <= '9':
			shade = agentNumberColor
		case token == "│" || token == "└" || token == "•":
			shade = muted
		default:
			shade = agentCommandColor
			bold = true
		}
		if bold {
			b.WriteString(strong(token, shade))
		} else {
			b.WriteString(ink(token, shade))
		}
		offset = span[1]
	}
	b.WriteString(ink(line[offset:], muted))
	return b.String()
}

func agentRenderToolBlock(sample string, width int) []string {
	var lines []string
	for _, line := range strings.Split(sample, "\n") {
		continuation := "    "
		if strings.HasPrefix(line, "• Ran ") || strings.HasPrefix(line, "  │ ") {
			continuation = "  │ "
		}
		for i, part := range strings.Split(wrapText(agentHighlightToolLine(line), max(1, width-4)), "\n") {
			if i > 0 {
				part = ink(continuation, muted) + part
			}
			lines = append(lines, part)
		}
	}
	return lines
}
