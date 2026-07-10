package bot

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatTextHTMLNestedEntities(t *testing.T) {
	plain, entities, err := FormatText(`<b>bold <i>text</i></b>`, "HTML", nil)
	require.NoError(t, err)
	require.Equal(t, "bold text", plain)
	require.Equal(t, []TextEntity{
		{Type: "bold", Offset: 0, Length: 9},
		{Type: "italic", Offset: 5, Length: 4},
	}, entities)
}

func TestFormatTextMarkdownV2Escaping(t *testing.T) {
	plain, entities, err := FormatText(`\*literal\* *bold* __under__ ||secret||`, "MarkdownV2", nil)
	require.NoError(t, err)
	require.Equal(t, "*literal* bold under secret", plain)
	require.Len(t, entities, 3)
	require.Equal(t, "bold", entities[0].Type)
	require.Equal(t, "underline", entities[1].Type)
	require.Equal(t, "spoiler", entities[2].Type)
}

func TestFormatTextHTMLLinkAndCustomEmoji(t *testing.T) {
	plain, entities, err := FormatText(
		`<a href="https://example.org">link</a> <tg-emoji emoji-id="42">👍</tg-emoji>`,
		"HTML",
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, "link 👍", plain)
	require.Equal(t, []TextEntity{
		{Type: "text_link", Offset: 0, Length: 4, URL: "https://example.org"},
		{Type: "custom_emoji", Offset: 5, Length: 2, CustomEmojiID: "42"},
	}, entities)
}

func TestFormatTextUsesUTF16Offsets(t *testing.T) {
	plain, entities, err := FormatText("😀 *bold*", "MarkdownV2", nil)
	require.NoError(t, err)
	require.Equal(t, "😀 bold", plain)
	require.Equal(t, []TextEntity{{Type: "bold", Offset: 3, Length: 4}}, entities)
}

func TestFormatTextExplicitEntitiesAreCanonicalAndKeepUTF8(t *testing.T) {
	plain, entities, err := FormatText("😀 text", "", []TextEntity{{
		Type:   " BOLD ",
		Offset: 3,
		Length: 4,
	}})
	require.NoError(t, err)
	require.Equal(t, "😀 text", plain)
	require.Equal(t, []TextEntity{{Type: "bold", Offset: 3, Length: 4}}, entities)

	plain, entities, err = FormatText(`\😀`, "MarkdownV2", nil)
	require.NoError(t, err)
	require.Equal(t, `\😀`, plain)
	require.Empty(t, entities)
}

func TestFormatTextMarkdownV2CustomEmojiLink(t *testing.T) {
	plain, entities, err := FormatText(`[👍](tg://emoji?id=42)`, "MarkdownV2", nil)
	require.NoError(t, err)
	require.Equal(t, "👍", plain)
	require.Equal(t, []TextEntity{{
		Type:          "custom_emoji",
		Offset:        0,
		Length:        2,
		CustomEmojiID: "42",
	}}, entities)
}

func TestFormatTextMarkdownSupportsEntitiesAndLinks(t *testing.T) {
	plain, entities, err := FormatText(
		"*bold* _italic_ __under__ ~strike~ ||secret|| `code` [link](https://example.org)",
		"Markdown",
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, "bold italic under strike secret code link", plain)
	require.Equal(t, []TextEntity{
		{Type: "bold", Offset: 0, Length: 4},
		{Type: "italic", Offset: 5, Length: 6},
		{Type: "underline", Offset: 12, Length: 5},
		{Type: "strikethrough", Offset: 18, Length: 6},
		{Type: "spoiler", Offset: 25, Length: 6},
		{Type: "code", Offset: 32, Length: 4},
		{Type: "text_link", Offset: 37, Length: 4, URL: "https://example.org"},
	}, entities)
}

func TestFormatTextRejectsInvalidMarkup(t *testing.T) {
	_, _, err := FormatText("<b>broken", "HTML", nil)
	require.ErrorIs(t, err, ErrInvalidInput)

	_, _, err = FormatText("*broken", "MarkdownV2", nil)
	require.ErrorIs(t, err, ErrInvalidInput)
}
