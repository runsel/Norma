package tool

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWebFetchSendsConfiguredIdentity pins the host-configurable identity: the
// tool must not invent a product User-Agent when a caller supplies one, and it
// must not set Accept-Encoding itself (that would disable transparent gzip).
func TestWebFetchSendsConfiguredIdentity(t *testing.T) {
	const wantUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	const wantLang = "pt-BR,pt;q=0.9,en;q=0.8"

	var gotUA, gotLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotLang = r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

	tl := NewWebFetch(WebFetchConfig{UserAgent: wantUA, AcceptLanguage: wantLang})
	raw, _ := json.Marshal(map[string]any{"url": srv.URL})
	res, err := tl.Call(context.Background(), raw, &ToolContext{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.IsError {
		t.Fatalf("fetch failed: %s", res.Flatten())
	}
	if gotUA != wantUA {
		t.Errorf("User-Agent = %q, want %q", gotUA, wantUA)
	}
	if gotLang != wantLang {
		t.Errorf("Accept-Language = %q, want %q", gotLang, wantLang)
	}
}

// TestWebFetchNoDirectFallbackByDefault pins the fail-closed default: a proxied
// failure must NOT silently retry directly unless the caller opts in, because a
// direct request is invisible to the recording proxy and any egress sanitizer.
func TestWebFetchNoDirectFallbackByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

	// Reserve a port, then close it: connecting to it is refused, which
	// proxyLikelyErr classifies as proxy-caused (i.e. retry-worthy).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	proxyURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	raw, _ := json.Marshal(map[string]any{"url": srv.URL})

	res, err := NewWebFetch(WebFetchConfig{Proxy: proxyURL}).Call(context.Background(), raw, &ToolContext{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("tool error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("proxied failure must not fall back direct by default: %s", res.Flatten())
	}

	res2, err := NewWebFetch(WebFetchConfig{Proxy: proxyURL, AllowDirectFallback: true}).Call(context.Background(), raw, &ToolContext{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("tool error: %v", err)
	}
	if res2.IsError {
		t.Fatalf("explicit AllowDirectFallback should have succeeded: %s", res2.Flatten())
	}
}
