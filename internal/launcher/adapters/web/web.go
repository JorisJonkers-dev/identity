// Package web is the Launcher's inbound HTTP adapter: getLauncher for the signed-in User, and the
// icons it names, served from identity's own origin so the page loads nothing from elsewhere.
package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/launcher/app"
	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

// iconPolicy lets an icon run nothing, even an SVG opened on its own.
const iconPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// UseCases is what the adapter needs from the Launcher.
type UseCases interface {
	Apps(ctx context.Context, userID uuid.UUID) ([]domain.App, error)
	HeldBack(ctx context.Context, userID uuid.UUID) (int, error)
	Icon(ctx context.Context, clientID string) (domain.Icon, error)
}

// Launcher implements getLauncher.
type Launcher struct{ uc UseCases }

// NewLauncher returns a Launcher calling uc.
func NewLauncher(uc UseCases) *Launcher { return &Launcher{uc: uc} }

// GetLauncher implements getLauncher. An app whose launch URL cannot be read is left out.
func (h *Launcher) GetLauncher(ctx context.Context) (oas.GetLauncherRes, error) {
	userID := principal.From(ctx).UserID
	apps, err := h.uc.Apps(ctx, userID)
	if err != nil {
		return nil, err
	}
	heldBack, err := h.uc.HeldBack(ctx, userID)
	if err != nil {
		return nil, err
	}
	res := &oas.Launcher{Apps: make([]oas.LauncherApp, 0, len(apps)), HeldBack: oas.Count(heldBack)} //nolint:gosec // a count of apps
	for _, a := range apps {
		launch, err := url.Parse(a.Launch())
		if err != nil {
			continue
		}
		out := oas.LauncherApp{ID: oas.ClientId(a.ID), Label: a.Label, LaunchUrl: *launch}
		if a.Description != "" {
			out.Description = oas.NewOptString(a.Description)
		}
		if a.IconURL != "" {
			out.IconUrl = oas.NewOptString(iconPath(a.Client))
		}
		for _, r := range a.Roles {
			out.Roles = append(out.Roles, oas.AppRole(r))
		}
		res.Apps = append(res.Apps, out)
	}
	return res, nil
}

// iconPath names the icon by its source too, so a browser drops a cached icon once it moves.
func iconPath(c domain.Client) string {
	sum := sha256.Sum256([]byte(c.IconURL))
	return "/icons/" + url.PathEscape(c.ID) + "?v=" + hex.EncodeToString(sum[:6])
}

type icons struct {
	uc     UseCases
	logger *slog.Logger
}

// NewIcons returns the handler for GET /icons/{clientId}. An icon needs no sign-in: it is the
// Client's own, public on its host.
func NewIcons(uc UseCases, logger *slog.Logger) http.Handler {
	h := &icons{uc: uc, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /icons/{clientId}", h.serve)
	return mux
}

func (h *icons) serve(w http.ResponseWriter, r *http.Request) {
	icon, err := h.uc.Icon(r.Context(), r.PathValue("clientId"))
	if err != nil {
		if err != app.ErrNoIcon { //nolint:errorlint // only a bare ErrNoIcon has no cause to log
			h.logger.WarnContext(r.Context(), "icon", "client", r.PathValue("clientId"), "error", err)
		}
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", icon.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Security-Policy", iconPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(icon.Body)
}
