package webui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/webui"
)

var app = fstest.MapFS{
	"index.html":        {Data: []byte("<!doctype html><title>app</title>")},
	"assets/app-abc.js": {Data: []byte("console.log(1)")},
	"favicon.svg":       {Data: []byte("<svg></svg>")},
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
	return rec
}

func TestServesIndexAtRootAndForClientRoutes(t *testing.T) {
	t.Parallel()
	h := webui.Handler(app)
	for _, p := range []string{"/", "/index.html", "/notes/42", "/settings"} {
		rec := get(h, p)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" || rec.Body.String() != "<!doctype html><title>app</title>" {
			t.Fatalf("%s: %d %q %q", p, rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
		}
	}
}

func TestServesHashedAssetsImmutable(t *testing.T) {
	t.Parallel()
	rec := get(webui.Handler(app), "/assets/app-abc.js")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset: %d %v", rec.Code, rec.Header())
	}
	if rec := get(webui.Handler(app), "/favicon.svg"); rec.Code != 200 || rec.Header().Get("Cache-Control") != "" {
		t.Fatalf("root file: %d %v", rec.Code, rec.Header())
	}
}

func TestMissingFilesWithExtensionAre404(t *testing.T) {
	t.Parallel()
	if rec := get(webui.Handler(app), "/assets/missing.js"); rec.Code != 404 {
		t.Fatalf("missing asset: %d", rec.Code)
	}
	if rec := get(webui.Handler(app), "/assets"); rec.Code != 200 {
		t.Fatalf("directory path falls back to the app: %d", rec.Code)
	}
}

func TestWithoutBuildExplains404(t *testing.T) {
	t.Parallel()
	rec := get(webui.Handler(fstest.MapFS{}), "/")
	if rec.Code != 404 {
		t.Fatalf("unbuilt: %d", rec.Code)
	}
}
