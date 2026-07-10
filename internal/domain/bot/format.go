package bot

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// FormatText removes Telegram markup and returns entities with UTF-16 offsets.
func FormatText(text string, parseMode string, entities []TextEntity) (string, []TextEntity, error) {
	mode := strings.ToLower(strings.TrimSpace(parseMode))
	switch mode {
	case "", "html", "markdown", "markdownv2":
	default:
		return "", nil, fmt.Errorf("unsupported parse mode %q: %w", parseMode, ErrInvalidInput)
	}
	if !utf8.ValidString(text) {
		return "", nil, ErrInvalidInput
	}
	if len(entities) > 0 {
		if mode != "" {
			return "", nil, ErrInvalidInput
		}
		result := append([]TextEntity(nil), entities...)
		for index := range result {
			result[index].Type = strings.ToLower(strings.TrimSpace(result[index].Type))
		}
		if err := validateTextEntities(text, result); err != nil {
			return "", nil, err
		}
		return text, result, nil
	}

	switch mode {
	case "":
		return text, nil, nil
	case "html":
		return formatHTML(text)
	default:
		return formatMarkdown(text, mode)
	}
}

func formattedTextMetadata(
	metadata map[string]string,
	text string,
	parseMode string,
	entities []TextEntity,
	metadataKey string,
) (string, map[string]string, error) {
	plainText, formattedEntities, err := FormatText(text, parseMode, entities)
	if err != nil {
		return "", nil, err
	}
	metadata, err = withTextEntities(metadata, metadataKey, formattedEntities)
	if err != nil {
		return "", nil, err
	}
	return plainText, metadata, nil
}

func validateTextEntities(text string, entities []TextEntity) error {
	boundaries := utf16Boundaries(text)
	max := len(utf16.Encode([]rune(text)))
	for _, entity := range entities {
		if !supportedTextEntity(entity.Type) || entity.Offset < 0 || entity.Length <= 0 {
			return ErrInvalidInput
		}
		end := entity.Offset + entity.Length
		if end > max || !boundaries[entity.Offset] || !boundaries[end] {
			return ErrInvalidInput
		}
		switch entity.Type {
		case "text_link":
			if strings.TrimSpace(entity.URL) == "" {
				return ErrInvalidInput
			}
		case "text_mention", "mention_name":
			if strings.TrimSpace(entity.UserID) == "" {
				return ErrInvalidInput
			}
		case "custom_emoji":
			if strings.TrimSpace(entity.CustomEmojiID) == "" {
				return ErrInvalidInput
			}
		}
	}

	return nil
}

func supportedTextEntity(entityType string) bool {
	switch strings.ToLower(strings.TrimSpace(entityType)) {
	case "mention", "hashtag", "cashtag", "bot_command", "url", "email", "phone_number",
		"bold", "italic", "underline", "strikethrough", "spoiler", "blockquote", "expandable_blockquote",
		"code", "pre", "text_link", "text_mention", "mention_name", "custom_emoji":
		return true
	default:
		return false
	}
}

func utf16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func utf16Boundaries(value string) map[int]bool {
	boundaries := map[int]bool{0: true}
	units := 0
	for _, r := range value {
		units += len(utf16.Encode([]rune{r}))
		boundaries[units] = true
	}
	return boundaries
}

func sortTextEntities(entities []TextEntity) {
	sort.SliceStable(entities, func(left, right int) bool {
		if entities[left].Offset != entities[right].Offset {
			return entities[left].Offset < entities[right].Offset
		}
		if entities[left].Length != entities[right].Length {
			return entities[left].Length > entities[right].Length
		}
		return entities[left].Type < entities[right].Type
	})
}

type htmlFrame struct {
	tag    string
	entity TextEntity
}

func formatHTML(input string) (string, []TextEntity, error) {
	var output strings.Builder
	frames := make([]htmlFrame, 0, 4)
	entities := make([]TextEntity, 0)

	for position := 0; position < len(input); {
		if input[position] != '<' {
			next := strings.IndexByte(input[position:], '<')
			if next < 0 {
				next = len(input) - position
			}
			output.WriteString(html.UnescapeString(input[position : position+next]))
			position += next
			continue
		}

		if strings.HasPrefix(input[position:], "<!--") {
			end := strings.Index(input[position+4:], "-->")
			if end < 0 {
				return "", nil, ErrInvalidInput
			}
			position += end + 7
			continue
		}

		end, err := htmlTagEnd(input, position+1)
		if err != nil {
			return "", nil, err
		}
		name, attributes, closing, selfClosing, err := parseHTMLTag(input[position+1 : end])
		if err != nil {
			return "", nil, err
		}
		position = end + 1

		if closing {
			if len(frames) == 0 || frames[len(frames)-1].tag != name {
				return "", nil, ErrInvalidInput
			}
			frame := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			frame.entity.Length = utf16Length(output.String()) - frame.entity.Offset
			if frame.entity.Length > 0 {
				entities = append(entities, frame.entity)
			}
			continue
		}

		if name == "br" {
			if !selfClosing {
				return "", nil, ErrInvalidInput
			}
			output.WriteByte('\n')
			continue
		}
		if selfClosing || !htmlEntityTag(name) {
			return "", nil, ErrInvalidInput
		}

		entity := TextEntity{
			Type:   htmlEntityType(name),
			Offset: utf16Length(output.String()),
		}
		switch name {
		case "a":
			entity.URL = attributes["href"]
			if strings.TrimSpace(entity.URL) == "" {
				return "", nil, ErrInvalidInput
			}
		case "tg-emoji":
			entity.CustomEmojiID = attributes["emoji-id"]
			if strings.TrimSpace(entity.CustomEmojiID) == "" {
				return "", nil, ErrInvalidInput
			}
		case "pre":
			entity.Language = htmlLanguage(attributes)
		case "code":
			entity.Language = htmlLanguage(attributes)
		}
		frames = append(frames, htmlFrame{tag: name, entity: entity})
	}
	if len(frames) != 0 {
		return "", nil, ErrInvalidInput
	}

	result := output.String()
	sortTextEntities(entities)
	return result, entities, nil
}

func htmlTagEnd(input string, start int) (int, error) {
	quote := byte(0)
	for position := start; position < len(input); position++ {
		switch input[position] {
		case '\'', '"':
			if quote == 0 {
				quote = input[position]
			} else if quote == input[position] {
				quote = 0
			}
		case '>':
			if quote == 0 {
				return position, nil
			}
		}
	}
	return 0, ErrInvalidInput
}

func parseHTMLTag(raw string) (string, map[string]string, bool, bool, error) {
	raw = strings.TrimSpace(raw)
	closing := strings.HasPrefix(raw, "/")
	if closing {
		raw = strings.TrimSpace(raw[1:])
	}
	selfClosing := strings.HasSuffix(raw, "/")
	if selfClosing {
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	}
	if raw == "" || strings.HasPrefix(raw, "!") || strings.HasPrefix(raw, "?") {
		return "", nil, false, false, ErrInvalidInput
	}

	position := 0
	for position < len(raw) && isHTMLSpace(raw[position]) {
		position++
	}
	start := position
	for position < len(raw) && !isHTMLSpace(raw[position]) && raw[position] != '=' {
		position++
	}
	name := strings.ToLower(raw[start:position])
	if name == "" {
		return "", nil, false, false, ErrInvalidInput
	}
	attributes := make(map[string]string)
	for position < len(raw) {
		for position < len(raw) && isHTMLSpace(raw[position]) {
			position++
		}
		if position == len(raw) {
			break
		}
		start = position
		for position < len(raw) && !isHTMLSpace(raw[position]) && raw[position] != '=' {
			position++
		}
		attributeName := strings.ToLower(raw[start:position])
		if attributeName == "" {
			return "", nil, false, false, ErrInvalidInput
		}
		for position < len(raw) && isHTMLSpace(raw[position]) {
			position++
		}
		if position >= len(raw) || raw[position] != '=' {
			return "", nil, false, false, ErrInvalidInput
		}
		position++
		for position < len(raw) && isHTMLSpace(raw[position]) {
			position++
		}
		if position >= len(raw) {
			return "", nil, false, false, ErrInvalidInput
		}
		quote := raw[position]
		if quote == '\'' || quote == '"' {
			position++
			valueStart := position
			for position < len(raw) && raw[position] != quote {
				position++
			}
			if position >= len(raw) {
				return "", nil, false, false, ErrInvalidInput
			}
			attributes[attributeName] = html.UnescapeString(raw[valueStart:position])
			position++
			continue
		}
		valueStart := position
		for position < len(raw) && !isHTMLSpace(raw[position]) {
			position++
		}
		attributes[attributeName] = html.UnescapeString(raw[valueStart:position])
	}

	return name, attributes, closing, selfClosing, nil
}

func isHTMLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func htmlEntityTag(name string) bool {
	switch name {
	case "b", "strong", "i", "em", "u", "s", "del", "code", "pre", "a", "blockquote", "tg-spoiler", "tg-emoji":
		return true
	default:
		return false
	}
}

func htmlEntityType(name string) string {
	switch name {
	case "b", "strong":
		return "bold"
	case "i", "em":
		return "italic"
	case "u":
		return "underline"
	case "s", "del":
		return "strikethrough"
	case "a":
		return "text_link"
	case "blockquote":
		return "blockquote"
	case "tg-spoiler":
		return "spoiler"
	case "tg-emoji":
		return "custom_emoji"
	default:
		return name
	}
}

func htmlLanguage(attributes map[string]string) string {
	if value := strings.TrimSpace(attributes["language"]); value != "" {
		return value
	}
	class := strings.TrimSpace(attributes["class"])
	return strings.TrimPrefix(class, "language-")
}

func formatMarkdown(input string, mode string) (string, []TextEntity, error) {
	output, entities, err := parseMarkdownSegment(input, mode)
	if err != nil {
		return "", nil, err
	}
	sortTextEntities(entities)
	return output, entities, nil
}

func parseMarkdownSegment(input string, mode string) (string, []TextEntity, error) {
	var output strings.Builder
	entities := make([]TextEntity, 0)
	for position := 0; position < len(input); {
		if input[position] == '\\' {
			if position+1 >= len(input) {
				return "", nil, ErrInvalidInput
			}
			next := input[position+1]
			if mode == "markdownv2" && !strings.ContainsRune("_*[]()~`>#+-=|{}.!\\", rune(next)) {
				output.WriteByte('\\')
				position++
				continue
			}
			output.WriteByte(next)
			position += 2
			continue
		}

		if strings.HasPrefix(input[position:], "```") {
			end := findMarkdownClose(input, position+3, "```")
			if end < 0 {
				return "", nil, ErrInvalidInput
			}
			language, body := markdownPreBody(input[position+3:end], mode)
			start := utf16Length(output.String())
			output.WriteString(body)
			entities = append(entities, TextEntity{Type: "pre", Offset: start, Length: utf16Length(body), Language: language})
			position = end + 3
			continue
		}
		if input[position] == '`' {
			end := findMarkdownClose(input, position+1, "`")
			if end < 0 {
				return "", nil, ErrInvalidInput
			}
			body := unescapeMarkdown(input[position+1:end], mode)
			start := utf16Length(output.String())
			output.WriteString(body)
			entities = append(entities, TextEntity{Type: "code", Offset: start, Length: utf16Length(body)})
			position = end + 1
			continue
		}
		if input[position] == '[' {
			labelEnd := findMarkdownClose(input, position+1, "]")
			if labelEnd < 0 || labelEnd+1 >= len(input) || input[labelEnd+1] != '(' {
				return "", nil, ErrInvalidInput
			}
			urlEnd := findMarkdownParenthesis(input, labelEnd+2)
			if urlEnd < 0 {
				return "", nil, ErrInvalidInput
			}
			label, nested, err := parseMarkdownSegment(input[position+1:labelEnd], mode)
			if err != nil {
				return "", nil, err
			}
			start := utf16Length(output.String())
			output.WriteString(label)
			for _, entity := range nested {
				entity.Offset += start
				entities = append(entities, entity)
			}
			url := strings.TrimSpace(unescapeMarkdown(input[labelEnd+2:urlEnd], mode))
			if url == "" {
				return "", nil, ErrInvalidInput
			}
			entity := TextEntity{
				Type:   "text_link",
				Offset: start,
				Length: utf16Length(label),
				URL:    url,
			}
			if customEmojiID(url) != "" {
				entity.Type = "custom_emoji"
				entity.CustomEmojiID = customEmojiID(url)
				entity.URL = ""
			}
			entities = append(entities, entity)
			position = urlEnd + 1
			continue
		}

		marker, entityType := markdownMarker(input[position:])
		if marker != "" {
			end := findMarkdownClose(input, position+len(marker), marker)
			if end < 0 || end == position+len(marker) {
				return "", nil, ErrInvalidInput
			}
			body, nested, err := parseMarkdownSegment(input[position+len(marker):end], mode)
			if err != nil {
				return "", nil, err
			}
			start := utf16Length(output.String())
			output.WriteString(body)
			for _, entity := range nested {
				entity.Offset += start
				entities = append(entities, entity)
			}
			entities = append(entities, TextEntity{Type: entityType, Offset: start, Length: utf16Length(body)})
			position = end + len(marker)
			continue
		}

		runeValue, size := utf8.DecodeRuneInString(input[position:])
		if runeValue == utf8.RuneError && size == 1 {
			return "", nil, ErrInvalidInput
		}
		output.WriteRune(runeValue)
		position += size
	}

	return output.String(), entities, nil
}

func markdownMarker(input string) (string, string) {
	switch {
	case strings.HasPrefix(input, "||"):
		return "||", "spoiler"
	case strings.HasPrefix(input, "__"):
		return "__", "underline"
	case strings.HasPrefix(input, "*"):
		return "*", "bold"
	case strings.HasPrefix(input, "_"):
		return "_", "italic"
	case strings.HasPrefix(input, "~"):
		return "~", "strikethrough"
	}
	return "", ""
}

func findMarkdownClose(input string, start int, marker string) int {
	for position := start; position <= len(input)-len(marker); position++ {
		if input[position] == '\\' {
			position++
			continue
		}
		if strings.HasPrefix(input[position:], marker) {
			return position
		}
	}
	return -1
}

func findMarkdownParenthesis(input string, start int) int {
	depth := 0
	for position := start; position < len(input); position++ {
		if input[position] == '\\' {
			position++
			continue
		}
		switch input[position] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return position
			}
			depth--
		}
	}
	return -1
}

func unescapeMarkdown(input string, mode string) string {
	var output strings.Builder
	for position := 0; position < len(input); position++ {
		if input[position] != '\\' || position+1 >= len(input) {
			output.WriteByte(input[position])
			continue
		}
		next := input[position+1]
		if mode != "markdownv2" || strings.ContainsRune("_*[]()~`>#+-=|{}.!\\", rune(next)) {
			output.WriteByte(next)
			position++
			continue
		}
		output.WriteByte('\\')
	}
	return output.String()
}

func customEmojiID(value string) string {
	const prefix = "tg://emoji?id="
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

func markdownPreBody(input string, mode string) (string, string) {
	input = unescapeMarkdown(input, mode)
	lineEnd := strings.IndexByte(input, '\n')
	if lineEnd <= 0 {
		return "", input
	}
	language := strings.TrimSpace(input[:lineEnd])
	if language == "" || strings.ContainsAny(language, " \t") {
		return "", input
	}
	return language, input[lineEnd+1:]
}

func encodeTextEntities(entities []TextEntity) (string, error) {
	if len(entities) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(entities)
	if err != nil {
		return "", fmt.Errorf("marshal text entities: %w", err)
	}
	return string(encoded), nil
}
