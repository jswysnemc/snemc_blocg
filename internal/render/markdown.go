package render

import (
	"bytes"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	renderhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type Heading struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

type Result struct {
	HTML        string
	TOC         []Heading
	Excerpt     string
	WordCount   int
	ReadingTime int
	PlainText   string
}

type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

func NewRenderer() *Renderer {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class", "id").OnElements("pre", "code", "span", "div", "h1", "h2", "h3", "h4", "h5", "h6")
	policy.AllowAttrs("data-language").OnElements("code")
	policy.AllowAttrs("data-language").OnElements("div")
	policy.AllowAttrs("src", "alt", "title", "loading", "decoding", "width", "height").OnElements("img")
	policy.AllowAttrs("class").OnElements("img")
	policy.AllowAttrs("src", "title", "allow", "allowfullscreen", "loading", "referrerpolicy", "frameborder").OnElements("iframe")
	policy.AllowURLSchemes("http", "https", "mailto")
	policy.RequireParseableURLs(true)
	policy.AllowStandardURLs()
	policy.AllowElements("figure", "figcaption")
	policy.AllowAttrs("class").OnElements("table", "thead", "tbody", "tr", "th", "td")

	return &Renderer{
		md: goldmark.New(
			goldmark.WithExtensions(
				extension.GFM,
				extension.Table,
				extension.Strikethrough,
				extension.TaskList,
				extension.Linkify,
				highlighting.NewHighlighting(
					highlighting.WithStyle("github-dark"),
					highlighting.WithFormatOptions(
						chromahtml.WithClasses(true),
						chromahtml.WithAllClasses(true),
						chromahtml.ClassPrefix("chroma-"),
					),
					highlighting.WithWrapperRenderer(renderCodeWrapper),
				),
			),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
			),
			goldmark.WithRendererOptions(
				renderhtml.WithUnsafe(),
			),
		),
		policy: policy,
	}
}

func (r *Renderer) Render(markdown string) (Result, error) {
	var out bytes.Buffer
	if err := r.md.Convert([]byte(markdown), &out); err != nil {
		return Result{}, err
	}

	rendered := out.String()
	rendered = wrapMermaidBlocks(rendered)
	rendered = r.policy.Sanitize(rendered)
	rendered = enhanceImageTags(rendered)

	plain := plainText(rendered)
	return Result{
		HTML:        rendered,
		TOC:         extractHeadings(rendered),
		Excerpt:     excerpt(plain, 160),
		WordCount:   countWords(plain),
		ReadingTime: readingTime(plain),
		PlainText:   plain,
	}, nil
}

func wrapMermaidBlocks(input string) string {
	re := regexp.MustCompile(`(?s)<div class="code-block" data-language="mermaid">.*?<pre[^>]*>\s*<code[^>]*(?:class="[^"]*language-mermaid[^"]*"|data-lang="mermaid")[^>]*>(.+?)</code>\s*</pre>\s*</div>`)
	return re.ReplaceAllStringFunc(input, func(match string) string {
		groups := re.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		code := decodeCodeHTML(groups[1])
		return `<div class="mermaid">` + code + `</div>`
	})
}

func decodeCodeHTML(input string) string {
	tagRe := regexp.MustCompile(`<[^>]+>`)
	withoutTags := tagRe.ReplaceAllString(input, "")
	return html.UnescapeString(withoutTags)
}

func enhanceImageTags(input string) string {
	imgRe := regexp.MustCompile(`(?i)<img\b[^>]*>`)
	srcRe := regexp.MustCompile(`\bsrc="([^"]+)"`)
	widthRe := regexp.MustCompile(`\bwidth="[^"]*"`)
	heightRe := regexp.MustCompile(`\bheight="[^"]*"`)
	loadingRe := regexp.MustCompile(`\bloading="[^"]*"`)
	decodingRe := regexp.MustCompile(`\bdecoding="[^"]*"`)

	return imgRe.ReplaceAllStringFunc(input, func(tag string) string {
		updated := tag
		srcMatch := srcRe.FindStringSubmatch(tag)
		if len(srcMatch) == 2 {
			if parsed, err := url.Parse(srcMatch[1]); err == nil && strings.HasPrefix(parsed.Path, "/media/") {
				query := parsed.Query()
				if width := parsePositiveAttribute(query.Get("w")); width > 0 && !widthRe.MatchString(updated) {
					updated = strings.Replace(updated, ">", ` width="`+strconv.Itoa(width)+`">`, 1)
				}
				if height := parsePositiveAttribute(query.Get("h")); height > 0 && !heightRe.MatchString(updated) {
					updated = strings.Replace(updated, ">", ` height="`+strconv.Itoa(height)+`">`, 1)
				}
				if parsed.RawQuery != "" {
					parsed.RawQuery = ""
					updated = strings.Replace(updated, `src="`+srcMatch[1]+`"`, `src="`+parsed.String()+`"`, 1)
				}
			}
		}
		if !loadingRe.MatchString(updated) {
			updated = strings.Replace(updated, ">", ` loading="lazy">`, 1)
		}
		if !decodingRe.MatchString(updated) {
			updated = strings.Replace(updated, ">", ` decoding="async">`, 1)
		}
		return updated
	})
}

func parsePositiveAttribute(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func renderCodeWrapper(w util.BufWriter, context highlighting.CodeBlockContext, entering bool) {
	language := "text"
	if value, ok := context.Language(); ok && len(value) > 0 {
		language = strings.ToLower(string(value))
	}
	escaped := html.EscapeString(language)
	if entering {
		if context.Highlighted() {
			_, _ = w.WriteString(`<div class="code-block" data-language="` + escaped + `">`)
			return
		}
		_, _ = w.WriteString(`<div class="code-block" data-language="` + escaped + `"><pre><code class="language-` + escaped + `">`)
		return
	}
	if !context.Highlighted() {
		_, _ = w.WriteString(`</code></pre></div>`)
		return
	}
	_, _ = w.WriteString(`</div>`)
}

func extractHeadings(input string) []Heading {
	re := regexp.MustCompile(`(?s)<h([2-4]) id="([^"]+)">(.+?)</h[2-4]>`)
	matches := re.FindAllStringSubmatch(input, -1)
	headings := make([]Heading, 0, len(matches))
	for _, match := range matches {
		level := int(match[1][0] - '0')
		headings = append(headings, Heading{
			Level: level,
			ID:    match[2],
			Text:  plainText(match[3]),
		})
	}
	return headings
}

func plainText(input string) string {
	tagRe := regexp.MustCompile(`<[^>]+>`)
	text := tagRe.ReplaceAllString(input, " ")
	text = html.UnescapeString(text)
	text = strings.Join(strings.Fields(text), " ")
	return text
}

func PlainTextHTML(input string) string {
	return plainText(input)
}

func excerpt(input string, limit int) string {
	if len([]rune(input)) <= limit {
		return input
	}
	runes := []rune(input)
	return string(runes[:limit]) + "..."
}

func countWords(input string) int {
	count := 0
	inWord := false
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				count++
				inWord = true
			}
			continue
		}
		if unicode.Is(unicode.Han, r) {
			count++
			inWord = false
			continue
		}
		inWord = false
	}
	if count == 0 && strings.TrimSpace(input) != "" {
		return len([]rune(input))
	}
	return count
}

func readingTime(input string) int {
	words := countWords(input)
	if words == 0 {
		return 1
	}
	if words < 240 {
		return 1
	}
	minutes := words / 240
	if words%240 != 0 {
		minutes++
	}
	return minutes
}
