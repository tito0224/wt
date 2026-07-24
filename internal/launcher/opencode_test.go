package launcher

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestOpencodeWebDeepLink(t *testing.T) {
	// A dir chosen so std base64 would need padding and produce '+' / '/';
	// the deep link must use unpadded base64url instead.
	dir := "/Users/me/proj>ects/wt?x"
	link := opencodeWebDeepLink("http://localhost:4096", dir)

	if !strings.HasPrefix(link, "http://localhost:4096/") {
		t.Fatalf("link %q missing origin prefix", link)
	}
	rest := strings.TrimPrefix(link, "http://localhost:4096/")
	b64, _, ok := strings.Cut(rest, "/session")
	if !ok {
		t.Fatalf("link %q missing /session segment", link)
	}
	if strings.ContainsAny(b64, "=+/") {
		t.Errorf("encoded dir %q contains padding or non-URL-safe characters", b64)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode %q: %v", b64, err)
	}
	if string(decoded) != dir {
		t.Errorf("decoded dir = %q; want %q", decoded, dir)
	}
	if !strings.Contains(link, "?prompt=") {
		t.Errorf("link %q missing prompt query", link)
	}
}

func TestSetOpencodeWebOrigin(t *testing.T) {
	defer SetOpencodeWebOrigin("") // restore the default (unset) state

	SetOpencodeWebOrigin("http://localhost:4096/ ")
	if got := opencodeWebOriginValue(); got != "http://localhost:4096" {
		t.Errorf("origin after trim = %q; want %q", got, "http://localhost:4096")
	}

	// Ensure the env fallback doesn't mask the cleared value.
	t.Setenv("OPENCODE_WEB_ORIGIN", "")
	os.Unsetenv("OPENCODE_WEB_ORIGIN")
	SetOpencodeWebOrigin("")
	if got := opencodeWebOriginValue(); got != "" {
		t.Errorf("origin after clearing = %q; want empty", got)
	}
}

func TestOpencodeWebKind(t *testing.T) {
	defer SetOpencodeWebOrigin("") // restore the default (unset) state
	t.Setenv("OPENCODE_WEB_ORIGIN", "")
	os.Unsetenv("OPENCODE_WEB_ORIGIN")

	SetOpencodeWebOrigin("")
	if got := (opencodeWeb{}).Kind(); got != KindTerminal {
		t.Errorf("Kind() without origin = %v; want KindTerminal", got)
	}

	SetOpencodeWebOrigin("http://localhost:4096")
	if got := (opencodeWeb{}).Kind(); got != KindURL {
		t.Errorf("Kind() with origin = %v; want KindURL", got)
	}
}
