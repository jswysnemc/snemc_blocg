package render

import (
	"strings"
	"testing"
)

func TestRendererPreservesMarkdownFeatures(t *testing.T) {
	renderer := NewRenderer()

	result, err := renderer.Render(strings.TrimSpace(`
- [ ] ~~todo~~

| Left | Center | Right |
| :--- | :---: | ---: |
| a | b | c |
`))
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	for _, fragment := range []string{
		`type="checkbox"`,
		`<del>todo</del>`,
		`align="center"`,
		`align="right"`,
	} {
		if !strings.Contains(result.HTML, fragment) {
			t.Fatalf("expected rendered HTML to contain %q, got: %s", fragment, result.HTML)
		}
	}
}

func TestRendererWrapsMermaidBlocks(t *testing.T) {
	renderer := NewRenderer()

	result, err := renderer.Render("```mermaid\ngraph TD\nA-->B\n```")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(result.HTML, `<div class="mermaid">`) {
		t.Fatalf("expected mermaid container, got: %s", result.HTML)
	}
	if strings.Contains(result.HTML, `data-language="mermaid"`) {
		t.Fatalf("expected mermaid code block wrapper to be removed, got: %s", result.HTML)
	}
}

func TestRendererHighlightsBashBlocks(t *testing.T) {
	renderer := NewRenderer()

	result, err := renderer.Render("```bash\necho hi\n```")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	for _, fragment := range []string{
		`data-language="bash"`,
		`class="chroma-`,
	} {
		if !strings.Contains(result.HTML, fragment) {
			t.Fatalf("expected rendered HTML to contain %q, got: %s", fragment, result.HTML)
		}
	}
}
