package output_test

import (
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/service/output"
)

func TestBuildEnvelope(t *testing.T) {
	env := output.BuildEnvelope("default", "en-FI", map[string]any{"ok": true}, nil, nil)
	if env.Meta["profile"] != "default" {
		t.Fatalf("expected profile default, got %v", env.Meta["profile"])
	}
	if env.Meta["locale"] != "en-FI" {
		t.Fatalf("expected locale en-FI, got %v", env.Meta["locale"])
	}
	requestID, _ := env.Meta["request_id"].(string)
	if !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("expected request_id prefix req_, got %q", requestID)
	}
	generatedAt, _ := env.Meta["generated_at"].(string)
	if !strings.HasSuffix(generatedAt, "Z") {
		t.Fatalf("expected generated_at to end with Z, got %q", generatedAt)
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("expected empty warnings, got %v", env.Warnings)
	}
}

func TestRenderPayload(t *testing.T) {
	env := output.BuildEnvelope("default", "en-FI", map[string]any{"ok": true}, []string{"warn"}, nil)

	jsonPayload, err := output.RenderPayload(env, output.FormatJSON)
	if err != nil {
		t.Fatalf("render json failed: %v", err)
	}
	if !strings.Contains(jsonPayload, "\"ok\": true") {
		t.Fatalf("expected json payload to include data, got %s", jsonPayload)
	}

	yamlPayload, err := output.RenderPayload(env, output.FormatYAML)
	if err != nil {
		t.Fatalf("render yaml failed: %v", err)
	}
	if !strings.Contains(yamlPayload, "profile: default") {
		t.Fatalf("expected yaml payload to include profile, got %s", yamlPayload)
	}
}
