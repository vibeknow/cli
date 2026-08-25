package cmdutil

import (
	"context"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/preview"
)

// assetResolver is the slice of figlens.Client this file needs. Declared as
// an interface so tests can deliver artifacts without a backend.
type assetResolver interface {
	AssetURL(ctx context.Context, ref string) (string, error)
}

// DeliverWorkArtifacts hands the caller local copies of whatever the work
// row currently points at: the cover still once the preview exists, and the
// MP4 once an export has produced one.
//
// Both are skipped when the field is empty, so this is safe to call at any
// point in a run — before the render finishes it simply delivers nothing.
// Calling it repeatedly is also safe: the deliverer compares content and
// stays quiet when the bytes have not changed.
//
// A nil deliverer (no --preview-dir) makes the whole thing a no-op,
// including the signing round-trips, so the default path pays nothing.
func DeliverWorkArtifacts(ctx context.Context, d *preview.Deliverer, c assetResolver, w *figlens.Work) {
	if d == nil || w == nil {
		return
	}
	deliver := func(kind preview.Kind, ref string) {
		if ref == "" {
			return
		}
		url, err := c.AssetURL(ctx, ref)
		if err != nil || url == "" {
			// Reported rather than dropped: from the caller's side an
			// artifact that could not be addressed and one that could not be
			// downloaded are the same event — "this did not arrive" — and
			// neither is a reason to fail a render that succeeded.
			d.Warn(kind, w.SessionID, "resolve_failed", err)
			return
		}
		d.Deliver(ctx, w.SessionID, kind, url)
	}
	deliver(preview.KindCover, w.CoverURL)
	deliver(preview.KindVideo, w.VideoPath)
}
