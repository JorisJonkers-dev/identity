package fetch_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/launcher/adapters/fetch"
)

// A 1x1 PNG.
var onePixel, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")

const svg = `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><rect width="1" height="1"/></svg>`

// serve answers every path with status, contentType and body over TLS, as a Client's host does.
func serve(t *testing.T, status int, contentType string, body []byte) (*httptest.Server, *fetch.Fetcher) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, fetch.New(srv.Client().Transport, time.Second)
}

func TestAPNGIconIsKeptAsAPNG(t *testing.T) {
	srv, f := serve(t, http.StatusOK, "image/png", onePixel)
	icon, err := f.Fetch(context.Background(), srv.URL+"/icon.png")
	if err != nil || icon.ContentType != "image/png" || !bytes.Equal(icon.Body, onePixel) {
		t.Fatalf("icon = %q %d bytes, %v", icon.ContentType, len(icon.Body), err)
	}
}

func TestTheIconsTypeComesFromItsBytesNotTheHeader(t *testing.T) {
	srv, f := serve(t, http.StatusOK, "application/octet-stream", onePixel)
	icon, err := f.Fetch(context.Background(), srv.URL+"/icon")
	if err != nil || icon.ContentType != "image/png" {
		t.Fatalf("icon = %q, %v", icon.ContentType, err)
	}
}

func TestAnSVGIconIsKeptAsAnSVG(t *testing.T) {
	srv, f := serve(t, http.StatusOK, "image/svg+xml; charset=utf-8", []byte(svg))
	icon, err := f.Fetch(context.Background(), srv.URL+"/icon.svg")
	if err != nil || icon.ContentType != "image/svg+xml" || string(icon.Body) != svg {
		t.Fatalf("icon = %q, %v", icon.ContentType, err)
	}
}

func TestAPageServedAsAnImageIsRefused(t *testing.T) {
	srv, f := serve(t, http.StatusOK, "image/png", []byte("<!doctype html><script>alert(1)</script>"))
	if _, err := f.Fetch(context.Background(), srv.URL+"/icon.png"); err == nil {
		t.Fatal("an HTML page was kept as an icon")
	}
}

func TestAnSVGWithoutAnSVGElementIsRefused(t *testing.T) {
	srv, f := serve(t, http.StatusOK, "image/svg+xml", []byte("<html><body>not an icon</body></html>"))
	if _, err := f.Fetch(context.Background(), srv.URL+"/icon.svg"); err == nil {
		t.Fatal("an HTML page labelled SVG was kept as an icon")
	}
}

func TestAnIconOverItsSizeLimitIsRefused(t *testing.T) {
	big := append(bytes.Clone(onePixel), bytes.Repeat([]byte{0}, 256<<10)...)
	srv, f := serve(t, http.StatusOK, "image/png", big)
	if _, err := f.Fetch(context.Background(), srv.URL+"/icon.png"); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnIconAtItsSizeLimitIsKept(t *testing.T) {
	exact := append(bytes.Clone(onePixel), bytes.Repeat([]byte{0}, 256<<10-len(onePixel))...)
	srv, f := serve(t, http.StatusOK, "image/png", exact)
	if _, err := f.Fetch(context.Background(), srv.URL+"/icon.png"); err != nil {
		t.Fatal(err)
	}
}

func TestAnAnswerOtherThan200IsRefused(t *testing.T) {
	srv, f := serve(t, http.StatusNotFound, "image/png", onePixel)
	if _, err := f.Fetch(context.Background(), srv.URL+"/icon.png"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v", err)
	}
}

func TestARedirectOnTheSameHostIsFollowed(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old.png" {
			http.Redirect(w, r, "/new.png", http.StatusFound)
			return
		}
		_, _ = w.Write(onePixel)
	}))
	defer srv.Close()
	if _, err := fetch.New(srv.Client().Transport, time.Second).Fetch(context.Background(), srv.URL+"/old.png"); err != nil {
		t.Fatal(err)
	}
}

func TestARedirectToAnotherHostIsRefused(t *testing.T) {
	elsewhere := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(onePixel) }))
	defer elsewhere.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/icon.png", http.StatusFound) // another port is another host
	}))
	defer srv.Close()
	if _, err := fetch.New(srv.Client().Transport, time.Second).Fetch(context.Background(), srv.URL+"/icon.png"); err == nil {
		t.Fatal("a redirect to another host was followed")
	}
}

func TestASlowHostTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	if _, err := fetch.New(srv.Client().Transport, 50*time.Millisecond).Fetch(context.Background(), srv.URL+"/icon.png"); err == nil {
		t.Fatal("a host that never answers did not time out")
	}
}

func TestAnUnreachableHostIsAnError(t *testing.T) {
	srv, f := serve(t, http.StatusOK, "image/png", onePixel)
	srv.Close()
	if _, err := f.Fetch(context.Background(), srv.URL+"/icon.png"); err == nil {
		t.Fatal("an unreachable host gave an icon")
	}
}

func TestABadURLIsAnError(t *testing.T) {
	_, f := serve(t, http.StatusOK, "image/png", onePixel)
	if _, err := f.Fetch(context.Background(), "://nowhere"); err == nil {
		t.Fatal("a bad URL gave an icon")
	}
}

func TestAnIconThatBreaksOffMidwayIsAnError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write(onePixel)
	}))
	defer srv.Close()
	if _, err := fetch.New(srv.Client().Transport, time.Second).Fetch(context.Background(), srv.URL+"/icon.png"); err == nil {
		t.Fatal("a cut-off icon was kept")
	}
}

func TestEveryRasterTypeABrowserShowsIsKept(t *testing.T) {
	for kind, magic := range map[string]string{
		"image/jpeg":   "\xff\xd8\xff\xe0",
		"image/gif":    "GIF89a",
		"image/webp":   "RIFF\x00\x00\x00\x00WEBPVP",
		"image/x-icon": "\x00\x00\x01\x00",
	} {
		t.Run(kind, func(t *testing.T) {
			srv, f := serve(t, http.StatusOK, "application/octet-stream", []byte(magic+"rest of the image"))
			icon, err := f.Fetch(context.Background(), srv.URL+"/icon")
			if err != nil || icon.ContentType != kind {
				t.Fatalf("icon = %q, %v", icon.ContentType, err)
			}
		})
	}
}
