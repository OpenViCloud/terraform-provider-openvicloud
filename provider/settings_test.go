package provider

import (
	"context"
	"strings"
	"testing"

	providerfw "github.com/hashicorp/terraform-plugin-framework/provider"
)

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestSettings_NothingSet_IsStagingAndAsksForTheToken(t *testing.T) {
	url, token, problem := Settings("", "", env(nil))

	if url != DefaultApiUrl {
		t.Fatalf("url = %q", url)
	}
	if token != "" || problem == "" || !strings.Contains(problem, "OPENVICLOUD_TOKEN") {
		t.Fatalf("token = %q, problem = %q", token, problem)
	}
}

func TestSettings_Environment_IsUsedWhenTheConfigurationSaysNothing(t *testing.T) {
	url, token, problem := Settings("", "", env(map[string]string{
		"OPENVICLOUD_API_URL": "http://api.localhost:8080", "OPENVICLOUD_TOKEN": " ovc_from_env ",
	}))

	if url != "http://api.localhost:8080" || token != "ovc_from_env" || problem != "" {
		t.Fatalf("url = %q, token = %q, problem = %q", url, token, problem)
	}
}

func TestSettings_Configuration_WinsOverTheEnvironment(t *testing.T) {
	url, token, _ := Settings("https://api.example", "ovc_from_config", env(map[string]string{
		"OPENVICLOUD_API_URL": "http://api.localhost:8080", "OPENVICLOUD_TOKEN": "ovc_from_env",
	}))

	if url != "https://api.example" || token != "ovc_from_config" {
		t.Fatalf("url = %q, token = %q", url, token)
	}
}

func TestSchema_TokenIsSensitive(t *testing.T) {
	var resp providerfw.SchemaResponse
	(&openViCloud{}).Schema(context.Background(), providerfw.SchemaRequest{}, &resp)

	token, ok := resp.Schema.Attributes["token"]
	if !ok || !token.IsSensitive() {
		t.Fatal("token must be a sensitive attribute: Terraform then hides it in plan output")
	}
}
