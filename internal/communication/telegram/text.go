package telegram

import "strings"

// maxMessage is the longest text sendMessage accepts.
const maxMessage = 4096

// Telegram rejects a sendMessage over maxMessage characters, so long answers
// are cut on the last blank line, newline or space that still fits.
func splitMessage(text string, limit int) []string {
	var parts []string
	for {
		runes := []rune(text)
		if len(runes) <= limit {
			if len(parts) == 0 || strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
			return parts
		}

		head := string(runes[:limit])
		cut := limit
		for _, separator := range []string{"\n\n", "\n", " "} {
			if index := strings.LastIndex(head, separator); index > 0 {
				cut = len([]rune(head[:index]))
				break
			}
		}
		parts = append(parts, strings.TrimRight(string(runes[:cut]), " \n"))
		text = strings.TrimLeft(string(runes[cut:]), " \n")
	}
}
