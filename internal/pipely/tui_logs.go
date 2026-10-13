package pipely

import "strings"

func styledLog(text string) string {
	lines := strings.Split(PrettyLog(text), "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		color := ""
		switch {
		case strings.Contains(lower, "##[error]") || strings.Contains(lower, `"level": "error"`):
			color = "#ff8c9b"
		case strings.Contains(lower, "##[warning]") || strings.Contains(lower, `"level": "warn"`):
			color = "#f4ce7b"
		case strings.Contains(lower, "##[section]"):
			color = "#b694ff"
		case strings.Contains(lower, "##[command]"):
			color = "#77d9dc"
		}
		for marker, label := range map[string]string{"##[error]": "ERROR  ", "##[warning]": "WARN  ", "##[section]": "", "##[command]": "$ "} {
			line = strings.ReplaceAll(line, marker, label)
		}
		lines[i] = hyperlinkText(line)
		if color != "" {
			lines[i] = "[" + color + "]" + lines[i] + "[-]"
		}
	}
	return strings.Join(lines, "\n")
}
