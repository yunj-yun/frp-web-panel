package caddy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminClientAppliesCaddyfileAfterAdapt(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.String())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"apps":{}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/adapt":
			if r.URL.Query().Get("adapter") != "caddyfile" {
				t.Error("adapt request did not select caddyfile adapter")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/load":
			if got := r.Header.Get("Content-Type"); got != "text/caddyfile" {
				t.Errorf("load content type = %q", got)
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewAdminClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(context.Background(), []byte(":443 { respond ok }")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Join(paths, "|") != "GET /config/|POST /adapt?adapter=caddyfile|POST /load?adapter=caddyfile" {
		t.Fatalf("unexpected admin calls: %v", paths)
	}
}

func TestAdminClientRollsBackWhenLoadFails(t *testing.T) {
	var loadBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"apps":{"http":{"servers":{}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/adapt":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/load":
			body, _ := io.ReadAll(r.Body)
			loadBodies = append(loadBodies, string(body))
			if len(loadBodies) == 1 {
				http.Error(w, "invalid config", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewAdminClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Apply(context.Background(), []byte(":443 { respond broken }"))
	if err == nil || !strings.Contains(err.Error(), "load Caddy configuration") {
		t.Fatalf("expected load error, got %v", err)
	}
	if len(loadBodies) != 2 || !strings.Contains(loadBodies[1], `"apps"`) {
		t.Fatalf("expected JSON rollback, got %q", loadBodies)
	}
}

func TestNewAdminClientRejectsInvalidURL(t *testing.T) {
	if _, err := NewAdminClient("not a URL", nil); err == nil {
		t.Fatal("invalid URL was accepted")
	}
}
