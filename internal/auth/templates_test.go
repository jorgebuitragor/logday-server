package auth

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// The panel's CSP is script-src/style-src 'self' with no 'unsafe-inline'
// (specs/cabeceras-seguridad/): any inline <script>, <style>, style=""
// or on*="" handler in a template would be silently blocked by the
// browser. This keeps them out.
func TestPanelTemplatesHaveNoInlineCode(t *testing.T) {
	inlineScript := regexp.MustCompile(`<script(\s[^>]*)?>`)
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`<style[\s>]`),
		regexp.MustCompile(`\sstyle\s*=`),
		regexp.MustCompile(`\son[a-z]+\s*=`),
	}
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := templateFS.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		for _, m := range inlineScript.FindAllString(src, -1) {
			if !strings.Contains(m, " src=") {
				t.Errorf("%s: inline <script> without src: %s", path, m)
			}
		}
		for _, re := range forbidden {
			if m := re.FindString(src); m != "" {
				t.Errorf("%s: inline code %q", path, strings.TrimSpace(m))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestServeStaticAllowlist(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.Get("/admin/static/{file}", h.serveStatic)

	for name, wantType := range map[string]string{
		"panel.css":     "text/css",
		"panel.js":      "text/javascript",
		"theme-init.js": "text/javascript",
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/static/"+name, nil))
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), wantType) || rec.Body.Len() == 0 {
			t.Errorf("%s: status %d, type %q, %d bytes", name, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
	}
	// Only the allowlisted names: nothing else embedded is reachable.
	for _, name := range []string{"logo.svg.bak", "..%2Ftemplates%2Fusers.html", "other.js"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/static/"+name, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, rec.Code)
		}
	}
}

// Cloudflare caches /admin/static/* for hours whatever the server says,
// keyed by full URL. Pages must ask for each static file with its
// content hash, so a deploy that changes a page and its script together
// can't be served the old script; and no template may skip {{asset}}.
func TestPanelTemplatesUseVersionedAssets(t *testing.T) {
	versions := assetVersions()
	tmpl := parseTemplates()
	var b strings.Builder
	if err := tmpl.ExecuteTemplate(&b, "login.html", formPageData{}); err != nil {
		t.Fatalf("rendering login.html: %v", err)
	}
	html := b.String()
	for _, name := range []string{"panel.css", "panel.js", "theme-init.js", "logo.svg"} {
		want := "/admin/static/" + name + "?v=" + versions[name]
		if !strings.Contains(html, want) {
			t.Errorf("login page should reference %s, got:\n%s", want, html)
		}
	}

	bare := regexp.MustCompile(`"/admin/static/[^"?{]+"`)
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := templateFS.ReadFile(path)
		if err != nil {
			return err
		}
		if m := bare.FindString(string(data)); m != "" {
			t.Errorf("%s: static file without {{asset}}: %s", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
