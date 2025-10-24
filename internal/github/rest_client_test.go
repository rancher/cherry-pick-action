package gh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTClientListPullRequestCommentsPagination(t *testing.T) {
	baseURL := ""
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v3/repos/rancher/repo/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}

		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "1":
			comments := []map[string]any{{"id": 1, "body": "first"}}
			next := baseURL + "/api/v3/repos/rancher/repo/issues/1/comments?per_page=100&page=2"
			w.Header().Set("Link", "<"+next+">; rel=\"next\", <"+next+">; rel=\"last\"")
			if err := json.NewEncoder(w).Encode(comments); err != nil {
				t.Fatalf("encode page 1 comments: %v", err)
			}
		case "2":
			comments := []map[string]any{{"id": 2, "body": "second"}}
			if err := json.NewEncoder(w).Encode(comments); err != nil {
				t.Fatalf("encode page 2 comments: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL = server.URL

	factory := NewRESTFactory(server.URL, server.URL)
	client, err := factory.New(context.Background(), "token")
	if err != nil {
		t.Fatalf("factory.New returned error: %v", err)
	}

	comments, err := client.ListPullRequestComments(context.Background(), "rancher", "repo", 1)
	if err != nil {
		t.Fatalf("ListPullRequestComments returned error: %v", err)
	}

	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].ID != 1 || comments[0].Body != "first" {
		t.Fatalf("unexpected first comment: %+v", comments[0])
	}
	if comments[1].ID != 2 || comments[1].Body != "second" {
		t.Fatalf("unexpected second comment: %+v", comments[1])
	}
}

func TestRESTClientUpdateComment(t *testing.T) {
	var recordedBody string

	handler := http.NewServeMux()
	handler.HandleFunc("/api/v3/repos/rancher/repo/issues/comments/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH method, got %s", r.Method)
		}
		defer func() {
			if err := r.Body.Close(); err != nil {
				t.Logf("failed to close request body: %v", err)
			}
		}()
		var payload struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		recordedBody = payload.Body

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"id": 42, "body": payload.Body}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	factory := NewRESTFactory(server.URL, server.URL)
	client, err := factory.New(context.Background(), "token")
	if err != nil {
		t.Fatalf("factory.New returned error: %v", err)
	}

	err = client.UpdateComment(context.Background(), "rancher", "repo", 42, "updated body")
	if err != nil {
		t.Fatalf("UpdateComment returned error: %v", err)
	}

	if recordedBody != "updated body" {
		t.Fatalf("expected body to be 'updated body', got %q", recordedBody)
	}
}

func TestRESTClientCheckOrgMembershipIsMember(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v3/orgs/rancher/memberships/alice", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET method, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"state": "active",
			"role":  "member",
			"user":  map[string]any{"login": "alice"},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	factory := NewRESTFactory(server.URL, server.URL)
	client, err := factory.New(context.Background(), "token")
	if err != nil {
		t.Fatalf("factory.New returned error: %v", err)
	}

	isMember, err := client.CheckOrgMembership(context.Background(), "rancher", "alice")
	if err != nil {
		t.Fatalf("CheckOrgMembership returned error: %v", err)
	}

	if !isMember {
		t.Fatalf("expected user to be a member")
	}
}

func TestRESTClientCheckOrgMembershipNotMember(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v3/orgs/rancher/memberships/bob", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET method, got %s", r.Method)
		}

		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"message": "Not Found",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	factory := NewRESTFactory(server.URL, server.URL)
	client, err := factory.New(context.Background(), "token")
	if err != nil {
		t.Fatalf("factory.New returned error: %v", err)
	}

	isMember, err := client.CheckOrgMembership(context.Background(), "rancher", "bob")
	if err != nil {
		t.Fatalf("CheckOrgMembership returned unexpected error: %v", err)
	}

	if isMember {
		t.Fatalf("expected user to not be a member")
	}
}
