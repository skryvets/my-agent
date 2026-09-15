package devcontainer

import "bytes"

// standardize turns JSON with comments, the format of devcontainer.json, into
// JSON that encoding/json reads.
func standardize(data []byte) []byte {
	return dropTrailingCommas(dropComments(data))
}

func dropComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			end := stringEnd(data, i)
			out = append(out, data[i:end]...)
			i = end - 1
			continue
		}
		if data[i] == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/':
				for i < len(data) && data[i] != '\n' {
					i++
				}
				if i < len(data) {
					out = append(out, '\n')
				}
				continue
			case '*':
				end := bytes.Index(data[i+2:], []byte("*/"))
				if end < 0 {
					return out
				}
				i += 2 + end + 1
				out = append(out, ' ')
				continue
			}
		}
		out = append(out, data[i])
	}
	return out
}

func dropTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			end := stringEnd(data, i)
			out = append(out, data[i:end]...)
			i = end - 1
			continue
		}
		if data[i] == ',' {
			next := i + 1
			for next < len(data) && bytes.IndexByte([]byte(" \t\r\n"), data[next]) >= 0 {
				next++
			}
			if next < len(data) && (data[next] == '}' || data[next] == ']') {
				continue
			}
		}
		out = append(out, data[i])
	}
	return out
}

// stringEnd is the index just after the string that opens at start.
func stringEnd(data []byte, start int) int {
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(data)
}
