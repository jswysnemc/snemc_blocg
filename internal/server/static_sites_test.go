package server

import (
	"strings"
	"testing"

	"github.com/snemc/snemc-blog/internal/store"
)

func TestRewriteStaticSiteTextRewritesUploadedRootPaths(t *testing.T) {
	input := []byte(`
		<link href="/assets/index.css">
		<script src="/assets/index.js"></script>
		fetch("/tutor1.zh_cn")
		background: url(/assets/logo.svg);
	`)

	output := string(rewriteStaticSiteText(input, "8n658jndpq", []string{
		"assets/index.css",
		"assets/index.js",
		"assets/logo.svg",
		"tutor1.zh_cn",
	}))

	for _, expected := range []string{
		`href="/h/8n658jndpq/assets/index.css"`,
		`src="/h/8n658jndpq/assets/index.js"`,
		`fetch("/h/8n658jndpq/tutor1.zh_cn")`,
		`url(/h/8n658jndpq/assets/logo.svg)`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected rewritten output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestRewriteStaticSiteTextOnlyRewritesUploadedPaths(t *testing.T) {
	input := []byte(`fetch("/api/data"); fetch("/tutor1.zh_cn")`)

	output := string(rewriteStaticSiteText(input, "8n658jndpq", []string{"tutor1.zh_cn"}))

	if strings.Contains(output, `/h/8n658jndpq/api/data`) {
		t.Fatalf("unexpected API path rewrite: %s", output)
	}
	if !strings.Contains(output, `fetch("/h/8n658jndpq/tutor1.zh_cn")`) {
		t.Fatalf("expected uploaded file path rewrite, got: %s", output)
	}
}

func TestExtractStaticSitePageTitle(t *testing.T) {
	title := extractStaticSitePageTitle([]staticSiteUploadFile{
		{
			RelativePath: "index.html",
			Data: []byte(`<!doctype html>
<html>
<head><title>  Benchmark &amp; Tutor  </title></head>
<body></body>
</html>`),
		},
	}, "index.html")

	if title != "Benchmark & Tutor" {
		t.Fatalf("expected parsed title, got %q", title)
	}
}

func TestStaticSiteDisplayTitlePrefersCachedPageTitle(t *testing.T) {
	title := staticSiteDisplayTitle(store.StaticSite{
		RouteID:      "8n658jndpq",
		DownloadName: "dist",
		PageTitle:    "解析后的页面标题",
	})

	if title != "解析后的页面标题" {
		t.Fatalf("expected cached page title, got %q", title)
	}
}
