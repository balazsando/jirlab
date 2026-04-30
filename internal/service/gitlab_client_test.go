package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TriggerPipelineRun
// ---------------------------------------------------------------------------

func TestTriggerPipelineRun_WithVariables(t *testing.T) {
	var capturedBody []byte
	var capturedMethod, capturedPath, capturedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedContentType = r.Header.Get("Content-Type")
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":42,"status":"pending","ref":"main","web_url":"https://gitlab.example.com/-/pipelines/42","project_id":123}`))
	}))
	defer server.Close()

	client := NewGitLabClient(server.URL, "test-token")
	p, err := client.TriggerPipelineRun(123, "main", map[string]string{"MY_VAR": "my_value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request shape
	if capturedMethod != http.MethodPost {
		t.Errorf("method: want POST, got %s", capturedMethod)
	}
	if capturedPath != "/projects/123/pipeline" {
		t.Errorf("path: want /projects/123/pipeline, got %s", capturedPath)
	}
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", capturedContentType)
	}

	// Verify JSON body structure
	var body struct {
		Ref       string `json:"ref"`
		Variables []struct {
			Key          string `json:"key"`
			Value        string `json:"value"`
			VariableType string `json:"variable_type"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body.Ref != "main" {
		t.Errorf("ref: want main, got %s", body.Ref)
	}
	if len(body.Variables) != 1 {
		t.Fatalf("variables length: want 1, got %d", len(body.Variables))
	}
	if body.Variables[0].Key != "MY_VAR" {
		t.Errorf("variable key: want MY_VAR, got %s", body.Variables[0].Key)
	}
	if body.Variables[0].Value != "my_value" {
		t.Errorf("variable value: want my_value, got %s", body.Variables[0].Value)
	}
	if body.Variables[0].VariableType != "env_var" {
		t.Errorf("variable_type: want env_var, got %s", body.Variables[0].VariableType)
	}

	// Verify parsed response
	if p.ID != 42 {
		t.Errorf("pipeline ID: want 42, got %d", p.ID)
	}
	if p.ProjectID != 123 {
		t.Errorf("project ID: want 123, got %d", p.ProjectID)
	}
	if p.WebURL != "https://gitlab.example.com/-/pipelines/42" {
		t.Errorf("web_url: want https://gitlab.example.com/-/pipelines/42, got %s", p.WebURL)
	}
}

func TestTriggerPipelineRun_NoVariables(t *testing.T) {
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":10,"status":"pending","ref":"develop","web_url":""}`))
	}))
	defer server.Close()

	client := NewGitLabClient(server.URL, "test-token")
	_, err := client.TriggerPipelineRun(1, "develop", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// variables field must be omitted when map is empty
	var body map[string]interface{}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := body["variables"]; exists {
		t.Error("variables field should be omitted when no variables are set")
	}
}

func TestTriggerPipelineRun_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"variables[0][variable_type] is invalid"}`))
	}))
	defer server.Close()

	client := NewGitLabClient(server.URL, "test-token")
	_, err := client.TriggerPipelineRun(1, "main", map[string]string{"FOO": "bar"})
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status 400, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetCILint — ref URL encoding
// ---------------------------------------------------------------------------

func TestGetCILint_RefURLEncoding(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"merged_yaml": "variables:\n  FOO:\n    value: bar\n    description: A variable\n",
			"valid":       true,
		})
	}))
	defer server.Close()

	client := NewGitLabClient(server.URL, "test-token")
	_, err := client.GetCILint(123, "feature/my-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// '/' must be percent-encoded in the query parameter value
	if !strings.Contains(capturedQuery, "ref=feature%2Fmy-branch") {
		t.Errorf("ref query param should be URL-encoded, got: %s", capturedQuery)
	}
}
