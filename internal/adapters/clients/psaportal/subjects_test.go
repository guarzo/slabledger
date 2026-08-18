package psaportal

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestFetchSubjects_PostsCategoryIDAndDecodesResult(t *testing.T) {
	routes := bundleRoutes()
	routes["immutable/chunks/REMOTE.js"] = `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign"),z=_t("abc123/getSubjects")`
	routes["/_app/remote/abc123/getSubjects"] = `{"type":"result","result":"[[1,2],{\"id\":22210,\"name\":\"Machamp\"},{\"id\":22301,\"name\":\"Charizard\"}]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	got, err := c.FetchSubjects(context.Background(), 16)
	if err != nil {
		t.Fatalf("FetchSubjects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != 22210 || got[0].Name != "Machamp" {
		t.Errorf("got[0] = %+v, want {22210 Machamp}", got[0])
	}
	if got[1].ID != 22301 || got[1].Name != "Charizard" {
		t.Errorf("got[1] = %+v, want {22301 Charizard}", got[1])
	}

	payloadStr := extractPayload(t, ff.captured["/_app/remote/abc123/getSubjects"])
	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	if string(decoded) != `[[1],16]` {
		t.Errorf("decoded ref-packed root = %s, want [[1],16]", decoded)
	}
}

func TestFetchSubjects_NonResultEnvelope(t *testing.T) {
	routes := bundleRoutes()
	routes["immutable/chunks/REMOTE.js"] = `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign"),z=_t("abc123/getSubjects")`
	routes["/_app/remote/abc123/getSubjects"] = `{"type":"error","message":"nope"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	if _, err := c.FetchSubjects(context.Background(), 16); err == nil {
		t.Fatal("expected error for non-result envelope")
	}
}
