// Package notetitle reads the title of a note from its markdown, as Apple Notes does: nobody types it apart.
package notetitle

import (
	"regexp"
	"strings"

	"github.com/cyril-fl/tide/core/services/hashtag"
)

// MaxLength is the most runes a title keeps when it comes from the first words of the text.
const MaxLength = 50

var (
	fence   = regexp.MustCompile("^[ \\t]*(```|~~~)")
	heading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	// block is what starts a line without being text: quote, list item, task box.
	block = regexp.MustCompile(`^[ \t]*(?:>[ \t]?)*(?:(?:[-*+]|\d+[.)])[ \t]+)?(?:\[[ xX]\][ \t]+)?`)
	image = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	link  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	wiki  = regexp.MustCompile(`\[\[([^\[\]\n]+)\]\]`)
	marks = regexp.MustCompile("\\*\\*|__|~~|[*_`]")
	rule  = regexp.MustCompile(`^[ \t]*(?:[-*_][ \t]*){3,}$`)
	// setext underlines the paragraph above as a heading: = level 1, - level 2.
	setext = regexp.MustCompile(`^ {0,3}(?:=+|-+)[ \t]*$`)
)

// From gives the first heading of content, else its first line of text cut to MaxLength runes;
// "" when there is neither: the note is untitled. Code blocks and lines of #tags only are skipped.
func From(content string) string {
	if title := Heading(content); title != "" {
		return title
	}
	lines := textLines(content)
	for _, line := range lines {
		if heading.MatchString(line) || rule.MatchString(line) {
			continue
		}
		if title := clean(block.ReplaceAllString(line, "")); title != "" {
			return cut(title)
		}
	}
	return ""
}

// Heading gives the first heading of content, "" without one.
func Heading(content string) string {
	if headings := Headings(content); len(headings) > 0 {
		return headings[0]
	}
	return ""
}

// Headings gives the headings of content, in order, as their words: those of fenced code and empty ones left out.
func Headings(content string) []string {
	var headings []string
	for _, line := range textLines(content) {
		if match := heading.FindStringSubmatch(line); match != nil {
			if title := clean(match[1]); title != "" {
				headings = append(headings, title)
			}
		}
	}
	return headings
}

// EmptySections gives the heading lines followed by nothing up to a heading of the same or a higher level, or the end
// of content: a section announced and never filled. Blank lines and lines of #tags only are nothing, fenced code is
// something (its # are not headings).
func EmptySections(content string) []string {
	type line struct {
		level int // 0: something that is not a heading
		text  string
	}
	var lines []line
	inCode := false
	paragraph := -1 // the index in lines of the paragraph an underline (=== or ---) would turn into a heading
	for raw := range strings.Lines(content) {
		raw = strings.TrimRight(raw, "\r\n")
		text := strings.TrimSpace(raw)
		switch {
		case fence.MatchString(raw):
			inCode = !inCode
			lines, paragraph = append(lines, line{}), -1
		case inCode:
			lines = append(lines, line{})
		case heading.MatchString(raw): // indented by 4 spaces or more, it is code
			lines, paragraph = append(lines, line{level: len(text) - len(strings.TrimLeft(text, "#")), text: text}), -1
		case paragraph >= 0 && setext.MatchString(raw):
			lines[paragraph].level = map[byte]int{'=': 1, '-': 2}[text[0]]
			lines, paragraph = lines[:paragraph+1], -1
		case text == "":
			paragraph = -1
		case strings.TrimSpace(hashtag.Remove(text, hashtag.Parse(text)...)) == "":
		case strings.TrimSpace(block.FindString(raw)) != "" || rule.MatchString(raw): // a list item, a quote or a rule ends a paragraph
			lines, paragraph = append(lines, line{}), -1
		case paragraph >= 0:
			lines[paragraph].text += " " + text
		case !strings.HasPrefix(raw, "    ") && !strings.HasPrefix(raw, "\t"):
			lines, paragraph = append(lines, line{text: text}), len(lines)
		default:
			lines = append(lines, line{})
		}
	}
	var empty []string
	for i, l := range lines {
		if l.level > 0 && (i == len(lines)-1 || lines[i+1].level > 0 && lines[i+1].level <= l.level) {
			empty = append(empty, l.text)
		}
	}
	return empty
}

// Helpers

// textLines drops fenced code: a # there is a comment, not a heading.
func textLines(content string) []string {
	var lines []string
	inCode := false
	for line := range strings.Lines(content) {
		if fence.MatchString(line) {
			inCode = !inCode
			continue
		}
		if !inCode {
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
	}
	return lines
}

// clean keeps the words: no link targets, [[ ]] around note links, emphasis marks or #tags.
func clean(text string) string {
	text = wiki.ReplaceAllString(text, "$1")
	text = image.ReplaceAllString(text, "$1")
	text = link.ReplaceAllString(text, "$1")
	text = marks.ReplaceAllString(text, "")
	text = hashtag.Remove(text, hashtag.Parse(text)...)
	return strings.Join(strings.Fields(text), " ")
}

func cut(title string) string {
	runes := []rune(title)
	if len(runes) <= MaxLength {
		return title
	}
	return strings.TrimSpace(string(runes[:MaxLength]))
}
