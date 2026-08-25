package preview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// collector captures emitted events in order.
type collector struct{ got []map[string]any }

func (c *collector) emit(ev map[string]any) { c.got = append(c.got, ev) }

func (c *collector) types() []string {
	out := make([]string, 0, len(c.got))
	for _, e := range c.got {
		out = append(out, e["type"].(string))
	}
	return out
}

func serve(t *testing.T, body *atomic.Value) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestDeliver_WritesAFileAndAnnouncesIt(t *testing.T) {
	var body atomic.Value
	body.Store("cover-bytes")
	srv := serve(t, &body)

	dir := t.TempDir()
	c := &collector{}
	d, err := New(dir, c.emit)
	if err != nil {
		t.Fatal(err)
	}

	d.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg")

	if len(c.got) != 1 || c.got[0]["type"] != TypeReady {
		t.Fatalf("want one %s event, got %v", TypeReady, c.types())
	}
	p, _ := c.got[0]["local_path"].(string)
	if !filepath.IsAbs(p) {
		t.Fatalf("local_path %q must be absolute — a relative path is meaningless to a caller with a different cwd", p)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("the announced path must exist: %v", err)
	}
	if string(got) != "cover-bytes" {
		t.Fatalf("file holds %q, want the downloaded bytes", got)
	}
	if c.got[0]["asset_kind"] != string(KindCover) {
		t.Fatalf("asset_kind = %v, want %v", c.got[0]["asset_kind"], KindCover)
	}
	// The remote address is a signed URL. An agent that relays it has
	// published a credential, so it must not be in the event at all.
	for k, v := range c.got[0] {
		if s, ok := v.(string); ok && strings.Contains(s, srv.URL) {
			t.Fatalf("field %q leaked the source URL: %q", k, s)
		}
	}
}

// Re-running a read-only command is supposed to be free. If it re-announced
// the same still every time, an agent polling status would show the user
// the same image on every poll.
func TestDeliver_IdenticalBytesAreNotAnnouncedTwice(t *testing.T) {
	var body atomic.Value
	body.Store("same")
	srv := serve(t, &body)

	dir := t.TempDir()
	c := &collector{}
	d, _ := New(dir, c.emit)

	d.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg")
	d.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg?sig=rotated")

	if len(c.got) != 1 {
		t.Fatalf("want 1 event, got %d (%v) — a rotated signature is not new content", len(c.got), c.types())
	}
}

// The dedupe survives the process, because the comparison is against the
// file on disk rather than in-memory state. A second CLI invocation into
// the same directory is the normal case, not the exotic one.
func TestDeliver_DedupeSurvivesANewDeliverer(t *testing.T) {
	var body atomic.Value
	body.Store("same")
	srv := serve(t, &body)
	dir := t.TempDir()

	first := &collector{}
	d1, _ := New(dir, first.emit)
	d1.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg")

	second := &collector{}
	d2, _ := New(dir, second.emit)
	d2.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg")

	if len(first.got) != 1 {
		t.Fatalf("first run should announce once, got %v", first.types())
	}
	if len(second.got) != 0 {
		t.Fatalf("second run re-announced unchanged content: %v", second.types())
	}
}

func TestDeliver_ChangedBytesAreAnnouncedAgain(t *testing.T) {
	var body atomic.Value
	body.Store("v1")
	srv := serve(t, &body)

	dir := t.TempDir()
	c := &collector{}
	d, _ := New(dir, c.emit)

	d.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg")
	body.Store("v2")
	d.Deliver(context.Background(), "sess_1", KindCover, srv.URL+"/x.jpg")

	if len(c.got) != 2 {
		t.Fatalf("want 2 events for 2 distinct versions, got %v", c.types())
	}
	p := c.got[1]["local_path"].(string)
	got, _ := os.ReadFile(p)
	if string(got) != "v2" {
		t.Fatalf("file holds %q after the second delivery, want v2", got)
	}
}

// A preview that will not download is a missing convenience. The video
// still rendered, so nothing here may fail the run.
func TestDeliver_DownloadFailureWarnsAndDoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &collector{}
	d, _ := New(t.TempDir(), c.emit)
	d.Deliver(context.Background(), "sess_1", KindVideo, srv.URL+"/x.mp4")

	if len(c.got) != 1 || c.got[0]["type"] != TypeWarning {
		t.Fatalf("want one %s, got %v", TypeWarning, c.types())
	}
	if c.got[0]["code"] != "download_failed" {
		t.Fatalf("code = %v, want a stable token a caller can branch on", c.got[0]["code"])
	}
}

func TestDeliver_PartialDownloadDoesNotReplaceAGoodFile(t *testing.T) {
	dir := t.TempDir()
	c := &collector{}
	d, _ := New(dir, c.emit)

	var body atomic.Value
	body.Store("good")
	ok := serve(t, &body)
	d.Deliver(context.Background(), "sess_1", KindCover, ok.URL+"/x.jpg")
	kept := c.got[0]["local_path"].(string)

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	d.Deliver(context.Background(), "sess_1", KindCover, bad.URL+"/x.jpg")

	got, err := os.ReadFile(kept)
	if err != nil || string(got) != "good" {
		t.Fatalf("a failed re-fetch destroyed the previously delivered file: %q %v", got, err)
	}
	// And nothing partial was left lying around next to it.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Fatalf("left a partial file behind: %s", e.Name())
		}
	}
}

func TestNew_EmptyDirIsOff(t *testing.T) {
	d, err := New("", func(map[string]any) {})
	if err != nil || d != nil {
		t.Fatalf("New(\"\") = (%v, %v), want (nil, nil)", d, err)
	}
	// Every method must tolerate it, or callers need a nil check each time.
	d.Deliver(context.Background(), "s", KindCover, "http://example.invalid/x")
	d.Warn(KindCover, "s", "x", nil)
	if d.Dir() != "" {
		t.Fatal("a disabled deliverer has no directory")
	}
}

// session_id comes from the backend, not from this process. A filename
// template is exactly the place where that distinction stops being academic.
func TestSlug_ContainsPathSeparators(t *testing.T) {
	cases := map[string]string{
		"sess_abc-123":     "sess_abc-123",
		"../../etc/passwd": "______etc_passwd",
		"":                 "run",
		"a/b":              "a_b",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Fatalf("slug(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.ContainsAny(slug("../x"), `/\`) {
		t.Fatal("slug output must never contain a path separator")
	}
	if len(slug(strings.Repeat("a", 500))) > 64 {
		t.Fatal("slug must bound its length; filesystems do not accept unbounded names")
	}
}

func TestExt(t *testing.T) {
	cases := []struct {
		url  string
		kind Kind
		want string
	}{
		{"https://x/y/cover.PNG", KindCover, ".png"},
		{"https://x/y/v.mp4?X-Amz-Signature=deadbeef", KindVideo, ".mp4"},
		{"https://x/y/nodot", KindCover, ".jpg"},
		{"https://x/y/nodot", KindVideo, ".mp4"},
		{"https://x/y/a.verylongextension", KindCover, ".jpg"},
	}
	for _, tc := range cases {
		if got := ext(tc.url, tc.kind); got != tc.want {
			t.Fatalf("ext(%q, %v) = %q, want %q", tc.url, tc.kind, got, tc.want)
		}
	}
}
