// Package preview delivers a run's visual artifacts as local files.
//
// Until now the only thing `vk create` handed back was a share_url — a
// hosted HTML page. A person can open it; an agent driving the CLI from a
// terminal cannot, so the one output of a video tool that is actually worth
// looking at was the one output an agent could not pass on. Everything an
// agent could show the user had to be fetched by a second command it had to
// know to run.
//
// A deliverer writes each artifact into a caller-named directory and
// announces it as a resource_ready event carrying an absolute path to a
// file that is already complete. That is the whole contract: the path is
// real, the bytes are all there, and the event fires once per distinct
// version of the asset.
//
// Deliberately absent from the event: the remote URL it came from. Those
// are signed, they expire, and an agent that logs one has published a
// credential. The local path is the only address a caller needs.
package preview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Kind labels what an artifact is, and becomes the event's asset_kind.
type Kind string

const (
	// KindCover is the still image the backend renders for the preview.
	KindCover Kind = "cover_image"
	// KindVideo is the exported MP4.
	KindVideo Kind = "video_playback"
)

// Event types written to the structured channel.
const (
	TypeReady   = "resource_ready"
	TypeWarning = "resource_preview_warning"
)

// Deliverer downloads artifacts into a directory and announces them.
// A nil *Deliverer is usable and does nothing, so callers that were not
// given a --preview-dir need no branch.
type Deliverer struct {
	dir  string
	emit func(map[string]any)
}

// New prepares a deliverer rooted at dir. An empty dir returns (nil, nil):
// the feature is off and every method becomes a no-op.
//
// dir may be absolute. Unlike `--dest`, which names a single file and is
// held to a relative path inside the working directory, this names a
// scratch directory the caller chose for itself — an agent's temp dir is a
// normal answer. What is untrusted here is not the directory but the
// backend-supplied id that forms each filename, so that is what gets
// sanitised (see slug).
func New(dir string, emit func(map[string]any)) (*Deliverer, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve --preview-dir %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create --preview-dir %q: %w", dir, err)
	}
	return &Deliverer{dir: abs, emit: emit}, nil
}

// Dir returns the absolute directory, or "" when delivery is off.
func (d *Deliverer) Dir() string {
	if d == nil {
		return ""
	}
	return d.dir
}

// Deliver fetches url into the preview directory and emits resource_ready
// when the result is something the caller has not been handed before.
//
// It never returns an error. A preview that fails to download is a missing
// convenience, not a failed run — the video still rendered — so the failure
// is reported as a warning event and the run carries on. This mirrors what
// the artifact is for: an aid to review, never evidence of the outcome.
func (d *Deliverer) Deliver(ctx context.Context, id string, kind Kind, url string) {
	if d == nil || strings.TrimSpace(url) == "" {
		return
	}
	local, changed, err := d.fetch(ctx, id, kind, url)
	if err != nil {
		d.warn(kind, id, err)
		return
	}
	if !changed {
		// Byte-identical to what is already on disk. Re-announcing it would
		// make an agent show the user the same still twice — once per poll
		// of a command that is meant to be safe to re-run.
		return
	}
	fi, statErr := os.Stat(local)
	ev := map[string]any{
		"type":       TypeReady,
		"asset_kind": string(kind),
		"session_id": id,
		"local_path": local,
	}
	if statErr == nil {
		ev["bytes"] = fi.Size()
	}
	d.event(ev)
}

// fetch downloads to a temp file in the same directory, compares it with
// whatever is already at the destination, and keeps it only if the bytes
// differ. Returns the destination path and whether it changed.
//
// Content comparison rather than a URL or timestamp check because the
// backend hands out freshly signed URLs for unchanged assets: keying on the
// address would re-deliver the same image on every call.
func (d *Deliverer) fetch(ctx context.Context, id string, kind Kind, url string) (string, bool, error) {
	dest := filepath.Join(d.dir, fmt.Sprintf("%s-%s%s", slug(id), kind, ext(url, kind)))

	tmp, err := os.CreateTemp(d.dir, ".preview-*.part")
	if err != nil {
		return "", false, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed away

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		tmp.Close()
		return "", false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tmp.Close()
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	sum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, sum), resp.Body); err != nil {
		tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}

	if prev, err := hashFile(dest); err == nil && prev == hex.EncodeToString(sum.Sum(nil)) {
		return dest, false, nil
	}
	// Rename last: until it succeeds the destination still holds the
	// previous complete file, so a caller reading the directory never sees
	// a partial one.
	if err := os.Rename(tmpPath, dest); err != nil {
		return "", false, err
	}
	return dest, true, nil
}

func (d *Deliverer) warn(kind Kind, id string, err error) {
	d.Warn(kind, id, "download_failed", err)
}

// Warn reports that an artifact the caller expected did not arrive, without
// failing anything. code is a stable token ("download_failed",
// "resolve_failed") so a consumer can branch without reading the message.
func (d *Deliverer) Warn(kind Kind, id, code string, err error) {
	if d == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	d.event(map[string]any{
		"type":       TypeWarning,
		"asset_kind": string(kind),
		"session_id": id,
		"code":       code,
		"message":    msg,
	})
}

func (d *Deliverer) event(ev map[string]any) {
	if d.emit == nil {
		return
	}
	d.emit(ev)
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// slug reduces a backend-supplied id to characters that are safe as one
// path component. A session_id is not user input, but it is not this
// process's input either, and `../` in a filename template is the kind of
// thing that only has to work once.
func slug(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return "run"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// ext picks a file extension from the URL, falling back to a per-kind
// default. Query strings are stripped first: signed URLs carry them, and
// `.jpg?X-Amz-Signature=…` is not an extension.
func ext(url string, kind Kind) string {
	clean := url
	if i := strings.IndexAny(clean, "?#"); i >= 0 {
		clean = clean[:i]
	}
	if e := path.Ext(clean); len(e) > 1 && len(e) <= 5 && !strings.ContainsAny(e, "/\\") {
		return strings.ToLower(e)
	}
	if kind == KindVideo {
		return ".mp4"
	}
	return ".jpg"
}
