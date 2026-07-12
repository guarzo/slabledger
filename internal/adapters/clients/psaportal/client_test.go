package psaportal

import (
	"testing"
)

func TestParseEmbedURL(t *testing.T) {
	p, j, err := parseEmbedURL("https://collectors.lightdash.cloud/embed/abc-123#tok.tok.tok")
	if err != nil || p != "abc-123" || j != "tok.tok.tok" {
		t.Fatalf("p=%q j=%q err=%v", p, j, err)
	}
	if _, _, err := parseEmbedURL("https://x/embed/abc-123"); err == nil {
		t.Fatal("expected error when missing #token")
	}
}
