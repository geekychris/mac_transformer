package core

import (
	"regexp"
	"strings"
)

// Chapter is one identified section of a document: a title and its body.
type Chapter struct {
	Number int
	Title  string
	Body   string
}

// headingRe recognises Markdown ATX headings, AsciiDoc `= Title`, and
// numbered chapter conventions ("Chapter 3: Foo"). Kept intentionally
// loose — false positives are rare in prose and false negatives just fall
// back to single-chapter output.
var (
	mdHeadingRe    = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)
	adocHeadingRe  = regexp.MustCompile(`(?m)^(=+)\s+(.+?)\s*$`)
	chapterWordRe  = regexp.MustCompile(`(?im)^\s*(chapter|part|section)\s+\d+(\s*[:\-–—]\s*(.*))?\s*$`)
	plainSepRe     = regexp.MustCompile(`(?m)^(?:={3,}|-{3,})\s*$`)
	firstLineSlice = 200
)

// DetectChapters walks plain (or lightly-formatted) text and returns one
// Chapter per detected section boundary. If nothing looks like a heading,
// returns a single-chapter list with the whole body.
//
// The detection is heuristic; callers should treat this as a suggestion,
// not a semantic guarantee. sourceExt hints at the source format so we
// can prefer format-native heuristics (# for md, = for adoc, etc.).
func DetectChapters(text, sourceExt string) []Chapter {
	// If source is md/adoc/asciidoc, use format-specific pass.
	switch strings.ToLower(sourceExt) {
	case "md", "markdown":
		if chs := splitByRegex(text, mdHeadingRe, 2); len(chs) > 1 {
			return chs
		}
	case "adoc", "asciidoc":
		if chs := splitByRegex(text, adocHeadingRe, 2); len(chs) > 1 {
			return chs
		}
	}
	// Try any of the heuristics; return whichever splits into more chapters.
	best := []Chapter{singleChapter(text)}
	for _, re := range []*regexp.Regexp{mdHeadingRe, adocHeadingRe, chapterWordRe, plainSepRe} {
		if chs := splitByRegex(text, re, 2); len(chs) > len(best) {
			best = chs
		}
	}
	return best
}

func splitByRegex(text string, re *regexp.Regexp, titleGroup int) []Chapter {
	matches := re.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var chapters []Chapter
	prevStart := 0
	prevTitle := "Preface"
	for i, m := range matches {
		if m[0] > prevStart {
			body := strings.TrimSpace(text[prevStart:m[0]])
			if body != "" {
				chapters = append(chapters, Chapter{
					Number: len(chapters) + 1,
					Title:  prevTitle,
					Body:   body,
				})
			}
		}
		// Extract title from the matched heading itself.
		title := "Chapter"
		if titleGroup*2+1 < len(m) && m[titleGroup*2] >= 0 {
			title = strings.TrimSpace(text[m[titleGroup*2]:m[titleGroup*2+1]])
		}
		prevTitle = title
		prevStart = m[1]

		// If we're the last match, capture the trailing body too.
		if i == len(matches)-1 {
			body := strings.TrimSpace(text[prevStart:])
			if body != "" {
				chapters = append(chapters, Chapter{
					Number: len(chapters) + 1,
					Title:  title,
					Body:   body,
				})
			}
		}
	}
	return chapters
}

func singleChapter(text string) Chapter {
	title := firstLineSummary(text)
	if title == "" {
		title = "Content"
	}
	return Chapter{Number: 1, Title: title, Body: strings.TrimSpace(text)}
}

// firstLineSummary grabs the first non-empty line as a rough title.
func firstLineSummary(text string) string {
	for _, ln := range strings.SplitN(text, "\n", firstLineSlice) {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		// Trim heading markers.
		ln = strings.TrimLeft(ln, "#= \t")
		if len(ln) > 80 {
			ln = ln[:80] + "…"
		}
		return ln
	}
	return ""
}
