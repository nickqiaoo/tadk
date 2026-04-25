package render

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func FormatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimRight(v, "\n")
	case []byte:
		return strings.TrimRight(string(v), "\n")
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func FormatToolCall(toolName string, args any) string {
	m := normalizedMap(args)
	if len(m) == 0 {
		return fallbackPreview(args, 6, 600)
	}

	switch toolName {
	case "bash":
		return formatBashCall(m)
	case "read_file":
		return formatReadFileCall(m)
	case "edit_file":
		return formatEditFileCall(m)
	case "write_file":
		return formatWriteFileCall(m)
	case "find_file":
		return formatFindFileCall(m)
	case "grep_search":
		return formatGrepCall(m)
	default:
		return formatGenericCall(m)
	}
}

func FormatToolCallDelta(toolName, delta string) string {
	_ = toolName
	if strings.TrimSpace(delta) == "" {
		return "preparing arguments…"
	}
	return "preparing arguments…"
}

func FormatToolResult(toolName string, result any, isError bool) string {
	m := normalizedMap(result)
	if len(m) == 0 {
		body := fallbackPreview(result, 10, 1200)
		if body == "" {
			if isError {
				return "error"
			}
			return "completed"
		}
		if isError {
			return "error\n" + body
		}
		return body
	}

	switch toolName {
	case "bash":
		return formatBashResult(m, isError)
	case "read_file":
		return formatReadFileResult(m, isError)
	case "edit_file":
		return formatEditWriteResult("updated", m, isError)
	case "write_file":
		return formatEditWriteResult("wrote", m, isError)
	case "find_file":
		return formatFindFileResult(m, isError)
	case "grep_search":
		return formatGrepResult(m, isError)
	default:
		return formatGenericResult(m, isError)
	}
}

func formatBashCall(args map[string]any) string {
	command := stringValue(args["command"])
	if command == "" {
		return formatGenericCall(args)
	}
	lines := []string{prefixCommandPreview(command)}
	if timeout := stringValue(args["timeout"]); timeout != "" {
		lines = append(lines, "timeout "+timeout)
	}
	return strings.Join(lines, "\n")
}

func formatReadFileCall(args map[string]any) string {
	path := firstString(args, "path", "file_path")
	if path == "" {
		return formatGenericCall(args)
	}
	line := displayPath(path)
	switch {
	case numberValue(args["pages"]) > 0:
		line += " · pages " + strconv.Itoa(numberValue(args["pages"]))
	case numberValue(args["offset"]) > 0 && numberValue(args["limit"]) > 0:
		start := numberValue(args["offset"])
		limit := numberValue(args["limit"])
		line += fmt.Sprintf(" · lines %d-%d", start, start+limit-1)
	case numberValue(args["offset"]) > 0:
		line += " · from line " + strconv.Itoa(numberValue(args["offset"]))
	}
	return line
}

func formatEditFileCall(args map[string]any) string {
	path := firstString(args, "path", "file_path")
	if path == "" {
		return formatGenericCall(args)
	}
	oldString := stringValue(args["old_string"])
	newString := stringValue(args["new_string"])
	lines := []string{displayPath(path)}
	switch {
	case oldString == "":
		lines = append(lines, "create file")
	case oldString != "" || newString != "":
		lines = append(lines, fmt.Sprintf("replace %s → %s", quotedPreview(oldString), quotedPreview(newString)))
	}
	return strings.Join(lines, "\n")
}

func formatWriteFileCall(args map[string]any) string {
	path := firstString(args, "path", "file_path")
	if path == "" {
		return formatGenericCall(args)
	}
	content := stringValue(args["content"])
	line := displayPath(path)
	if content != "" {
		line += fmt.Sprintf(" · %d lines", lineCount(content))
	}
	return line
}

func formatFindFileCall(args map[string]any) string {
	path := firstString(args, "path", "dir", "directory")
	pattern := firstString(args, "pattern", "glob")
	if path == "" {
		return formatGenericCall(args)
	}
	if pattern == "" {
		pattern = "**"
	}
	line := pattern + " in " + displayPath(path)
	if maxResults := numberValue(args["max_results"]); maxResults > 0 {
		line += " · max " + strconv.Itoa(maxResults)
	}
	return line
}

func formatGrepCall(args map[string]any) string {
	pattern := firstString(args, "pattern", "query")
	path := firstString(args, "path", "dir", "directory")
	if pattern == "" && path == "" {
		return formatGenericCall(args)
	}
	lines := []string{}
	if pattern != "" {
		lines = append(lines, `pattern `+quotedPreview(pattern))
	}
	if path != "" {
		lines = append(lines, "in "+displayPath(path))
	}
	if filePattern := stringValue(args["file_pattern"]); filePattern != "" {
		lines = append(lines, "files "+filePattern)
	}
	return strings.Join(lines, "\n")
}

func formatGenericCall(args map[string]any) string {
	parts := make([]string, 0, 4)
	for _, key := range []string{"path", "file_path", "pattern", "query", "command", "timeout"} {
		if value := stringValue(args[key]); value != "" {
			if key == "command" {
				return prefixCommandPreview(value)
			}
			parts = append(parts, key+" "+previewText(value, 1, 80))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	return fallbackPreview(args, 6, 600)
}

func formatBashResult(result map[string]any, isError bool) string {
	exitCode, hasExit := intValue(result["exit_code"])
	output := stringValue(result["output"])
	errText := stringValue(result["error"])

	header := "completed"
	switch {
	case hasExit && exitCode == 0:
		header = "exit 0"
	case hasExit:
		header = fmt.Sprintf("exit %d", exitCode)
	case isError:
		header = "error"
	}
	if errText != "" {
		header += " · " + previewText(errText, 1, 120)
	}

	if strings.TrimSpace(output) == "" {
		return header
	}
	return header + "\n" + previewText(output, 8, 800)
}

func formatReadFileResult(result map[string]any, isError bool) string {
	if isError {
		return formatGenericResult(result, true)
	}
	path := firstString(result, "path", "file_path")
	content := stringValue(result["content"])
	size, _ := intValue(result["size"])

	header := "read file"
	if path != "" {
		header = "read " + displayPath(path)
	}

	parts := make([]string, 0, 2)
	if content != "" {
		parts = append(parts, fmt.Sprintf("%d lines", lineCount(content)))
	}
	if size > 0 {
		parts = append(parts, humanBytes(size))
	}
	if len(parts) > 0 {
		header += " · " + strings.Join(parts, " · ")
	}
	return header
}

func formatEditWriteResult(verb string, result map[string]any, isError bool) string {
	if isError {
		return formatGenericResult(result, true)
	}
	path := firstString(result, "path", "file_path")
	if path == "" {
		return verb
	}
	return verb + " " + displayPath(path)
}

func formatFindFileResult(result map[string]any, isError bool) string {
	if isError {
		return formatGenericResult(result, true)
	}
	files := stringSlice(result["files"])
	count := len(files)
	if explicitCount := numberValue(result["count"]); explicitCount > count {
		count = explicitCount
	}
	header := fmt.Sprintf("%d file", count)
	if count != 1 {
		header += "s"
	}
	if truthy(result["truncated"]) {
		header += " · truncated"
	}
	if len(files) == 0 {
		return header
	}
	return header + "\n" + joinPreview(files, 5)
}

func formatGrepResult(result map[string]any, isError bool) string {
	if isError {
		return formatGenericResult(result, true)
	}
	matches := grepMatchSummaries(result["matches"])
	count := len(matches)
	if explicitCount := numberValue(result["count"]); explicitCount > count {
		count = explicitCount
	}
	header := fmt.Sprintf("%d match", count)
	if count != 1 {
		header += "es"
	}
	if truthy(result["truncated"]) {
		header += " · truncated"
	}
	if len(matches) == 0 {
		return header
	}
	return header + "\n" + joinPreview(matches, 4)
}

func formatGenericResult(result map[string]any, isError bool) string {
	if errText := stringValue(result["error"]); errText != "" {
		return "error\n" + previewText(errText, 6, 500)
	}
	if truthy(result["success"]) {
		if path := firstString(result, "path", "file_path"); path != "" {
			return "completed · " + displayPath(path)
		}
		return "completed"
	}
	if output := stringValue(result["output"]); output != "" {
		return previewText(output, 8, 800)
	}
	body := fallbackPreview(result, 8, 1000)
	if body == "" {
		if isError {
			return "error"
		}
		return "completed"
	}
	if isError {
		return "error\n" + body
	}
	return body
}

func normalizedMap(value any) map[string]any {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return ""
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func numberValue(value any) int {
	n, _ := intValue(value)
	return n
}

func intValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text := stringValue(item); text != "" {
				out = append(out, displayPath(text))
			}
		}
		return out
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var arr []string
		if err := json.Unmarshal(data, &arr); err == nil {
			for i := range arr {
				arr[i] = displayPath(arr[i])
			}
			return arr
		}
		return nil
	}
}

func grepMatchSummaries(value any) []string {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		file := displayPath(firstString(row, "file", "path"))
		line := numberValue(row["line"])
		content := previewText(stringValue(row["content"]), 1, 120)
		switch {
		case file != "" && line > 0 && content != "":
			out = append(out, fmt.Sprintf("%s:%d  %s", file, line, content))
		case file != "" && line > 0:
			out = append(out, fmt.Sprintf("%s:%d", file, line))
		case file != "":
			out = append(out, file)
		}
	}
	return out
}

func prefixCommandPreview(command string) string {
	lines := strings.Split(previewText(command, 2, 160), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = "$ " + lines[i]
		} else {
			lines[i] = "  " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func previewText(s string, maxLines, maxChars int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	truncated := false
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	text := strings.Join(lines, "\n")
	if maxChars > 0 && len([]rune(text)) > maxChars {
		text = string([]rune(text)[:maxChars])
		truncated = true
	}
	if truncated {
		text = strings.TrimRight(text, "\n ") + "…"
	}
	return text
}

func quotedPreview(s string) string {
	if strings.TrimSpace(s) == "" {
		return `""`
	}
	return strconv.Quote(previewText(s, 1, 40))
}

func joinPreview(items []string, maxItems int) string {
	if len(items) == 0 {
		return ""
	}
	if maxItems <= 0 || len(items) <= maxItems {
		return strings.Join(items, "\n")
	}
	return strings.Join(items[:maxItems], "\n") + fmt.Sprintf("\n… %d more", len(items)-maxItems)
}

func lineCount(s string) int {
	s = strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func humanBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

func displayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if len(clean) <= 72 {
		return clean
	}
	base := filepath.Base(clean)
	parent := filepath.Base(filepath.Dir(clean))
	if parent == "." || parent == string(filepath.Separator) {
		return "…/" + base
	}
	return "…/" + parent + "/" + base
}

func fallbackPreview(value any, maxLines, maxChars int) string {
	return previewText(FormatValue(value), maxLines, maxChars)
}
