package blogembed

import (
	"embed"
	"io/fs"
)

//go:embed web/templates/*.gohtml web/assets/* frontend/dist/* frontend/dist/assets/*
var embeddedFiles embed.FS

var (
	TemplatesFS    = mustSubFS("web/templates")
	PublicAssetsFS = mustSubFS("web/assets")
	FrontendDistFS = mustSubFS("frontend/dist")
)

func mustSubFS(path string) fs.FS {
	sub, err := fs.Sub(embeddedFiles, path)
	if err != nil {
		panic(err)
	}
	return sub
}
