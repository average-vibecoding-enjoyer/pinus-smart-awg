package smart

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const syntheticImportToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestImportLinkTransportAndCommandRejection(t *testing.T) {
	link, err := ParseImportLink("https://example.test/pinus/import#token=" + syntheticImportToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range []string{"", "PreUp = echo unsafe\n", "PostUp = echo unsafe\n", "PreDown = echo unsafe\n", "PostDown = echo unsafe\n"} {
		t.Run(strings.TrimSpace(hook), func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://example.test/api/pinus/import" || req.URL.Fragment != "" || req.Method != "POST" || req.Header.Get("Authorization") != "Bearer "+syntheticImportToken {
					t.Fatal("token transport contract violated")
				}
				body, _ := json.Marshal(importPayload{Version: 1, Name: "Synthetic", Config: strings.Replace(dualStackTestProfile, "[Interface]\n", "[Interface]\n"+hook, 1)})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			config, err := claimImportLink(context.Background(), link, client)
			if hook == "" {
				if err != nil || config == nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("remote execution hook accepted")
			}
		})
	}
}
func TestImportLinkRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"http://example.test/pinus/import#token=", "https://user:pw@example.test/pinus/import#token=", "https://example.test:8443/pinus/import#token=", "https://example.test/pinus/import?token=x#token=", "https://example.test/anything#token=", "https://example.test/pinus/import#"} {
		if _, err := ParseImportLink(raw + syntheticImportToken); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
