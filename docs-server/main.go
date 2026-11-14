// Command docs-server serves the static integration Markdown files as HTML.
// It lets teams browse "how my app plugs into the Provengo telemetry stack"
// from a browser instead of reading raw .md files.
package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

func main() {
	docsDir := env("DOCS_DIR", "/docs")
	addr := env("DOCS_ADDR", ":8081")

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Index: list the available markdown guides.
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		files, err := listMarkdown(docsDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var links []link
		for _, f := range files {
			links = append(links, link{Href: "/" + f, Text: titleize(f)})
		}
		render(w, "Provengo Telemetry — Integration Docs", indexBody(links))
	})

	// Serve a single markdown file rendered to HTML.
	r.Get("/{name}", func(w http.ResponseWriter, req *http.Request) {
		name := chi.URLParam(req, "name")
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		// Prevent path traversal: only a bare filename is allowed.
		if name != filepath.Base(name) {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		raw, err := os.ReadFile(filepath.Join(docsDir, name))
		if err != nil {
			http.NotFound(w, req)
			return
		}
		var buf bytes.Buffer
		if err := md.Convert(raw, &buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, titleize(name), template.HTML(buf.String()))
	})

	log.Printf("docs-server listening on %s (serving %s)", addr, docsDir)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

type link struct {
	Href string
	Text string
}

func indexBody(links []link) template.HTML {
	var b strings.Builder
	b.WriteString("<h1>Provengo Telemetry — Integration Docs</h1>")
	b.WriteString("<p>Pick a guide to see how to connect a service to the stack.</p><ul>")
	for _, l := range links {
		b.WriteString("<li><a href=\"" + template.HTMLEscapeString(l.Href) + "\">" +
			template.HTMLEscapeString(l.Text) + "</a></li>")
	}
	b.WriteString("</ul>")
	return template.HTML(b.String())
}

func listMarkdown(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func titleize(name string) string {
	n := strings.TrimSuffix(name, ".md")
	n = strings.ReplaceAll(n, "_", " ")
	n = strings.ReplaceAll(n, "-", " ")
	return n
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
 :root{color-scheme:light dark}
 body{max-width:820px;margin:2rem auto;padding:0 1rem;font:16px/1.6 -apple-system,Segoe UI,Roboto,sans-serif}
 a{color:#2563eb;text-decoration:none}a:hover{text-decoration:underline}
 pre{background:#0d1117;color:#e6edf3;padding:1rem;border-radius:8px;overflow:auto}
 code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
 :not(pre)>code{background:#8882;padding:.1em .35em;border-radius:4px}
 table{border-collapse:collapse}th,td{border:1px solid #8884;padding:.4rem .6rem}
 nav{margin-bottom:1.5rem;font-size:.9rem}
 h1,h2,h3{line-height:1.25}
</style></head><body>
<nav><a href="/">&larr; All guides</a></nav>
{{.Body}}
</body></html>`))

func render(w http.ResponseWriter, title string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, struct {
		Title string
		Body  template.HTML
	}{title, body})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
