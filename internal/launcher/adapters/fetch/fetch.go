// Package fetch is the Launcher's outbound adapter: it fetches a Client's icon from the Client's
// own host and keeps it only when its bytes are an image.
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/launcher/app"
	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
)

// maxIcon is the largest icon identity keeps.
const maxIcon = 256 << 10

// raster reports whether identity serves an image type, as http.DetectContentType names it.
func raster(kind string) bool {
	switch kind {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon":
		return true
	}
	return false
}

var errOffHost = errors.New("a redirect left the Client's host")

// Fetcher is the app.Fetcher.
type Fetcher struct{ client *http.Client }

var _ app.Fetcher = (*Fetcher)(nil)

// New returns a Fetcher over transport that gives up after timeout and follows redirects only on
// the same host.
func New(transport http.RoundTripper, timeout time.Duration) *Fetcher {
	return &Fetcher{client: &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != via[0].URL.Host {
				return errOffHost
			}
			return nil
		},
	}}
}

// Fetch fetches url and returns its body when it is a raster image or an SVG.
func (f *Fetcher) Fetch(ctx context.Context, url string) (domain.Icon, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.Icon{}, err
	}
	res, err := f.client.Do(req)
	if err != nil {
		return domain.Icon{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return domain.Icon{}, fmt.Errorf("the Client answered %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxIcon+1))
	if err != nil {
		return domain.Icon{}, err
	}
	if len(body) > maxIcon {
		return domain.Icon{}, fmt.Errorf("the icon is larger than %d bytes", maxIcon)
	}
	kind, err := imageType(res.Header.Get("Content-Type"), body)
	if err != nil {
		return domain.Icon{}, err
	}
	return domain.Icon{ContentType: kind, Body: body}, nil
}

// imageType is the icon's type: a raster type its bytes show, or SVG when the Client says so and
// the body holds an svg element.
func imageType(header string, body []byte) (string, error) {
	sniffed := http.DetectContentType(body)
	if raster(sniffed) {
		return sniffed, nil
	}
	declared, _, _ := mime.ParseMediaType(header)
	if declared == "image/svg+xml" && bytes.Contains(body, []byte("<svg")) {
		return declared, nil
	}
	return "", fmt.Errorf("the icon is not an image: %s", sniffed)
}
