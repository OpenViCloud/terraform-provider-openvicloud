package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_SendsTheTokenAndTheProviderVersion(t *testing.T) {
	var authorization, userAgent, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		userAgent = r.Header.Get("User-Agent")
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := New(server.URL+"/", "ovc_token", "1.2.3", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.TitanInfrastructureEndpointsCatalogListClustersEndpointWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if authorization != "Bearer ovc_token" {
		t.Fatalf("Authorization = %q", authorization)
	}
	if path != "/v1/clusters" {
		t.Fatalf("path = %q (an api_url ending in / must not give //v1)", path)
	}
	if userAgent != "terraform-provider-openvicloud/1.2.3" {
		t.Fatalf("User-Agent = %q", userAgent)
	}
}

func TestProblemOf_ReadsTheErrorBody(t *testing.T) {
	problem := ProblemOf(&http.Response{StatusCode: 404}, []byte(`{"status":404,"title":"Cluster x is not configured.","code":"CLUSTER_NOT_FOUND"}`))

	if problem.Code != "CLUSTER_NOT_FOUND" || problem.Status != 404 {
		t.Fatalf("problem = %+v", problem)
	}
	if problem.Error() != "HTTP 404 CLUSTER_NOT_FOUND: Cluster x is not configured." {
		t.Fatalf("Error() = %q", problem.Error())
	}
}

func TestProblemOf_NotAProblem_StillHasTheStatus(t *testing.T) {
	problem := ProblemOf(&http.Response{StatusCode: 401}, nil)

	if problem.Status != 401 || problem.Title != "Unauthorized" || problem.Code != "" {
		t.Fatalf("problem = %+v", problem)
	}
}

func TestExplain_Unauthorized_SaysTheTokenWasRefused(t *testing.T) {
	message := Explain(&Problem{Status: 401, Title: "Unauthorized"})

	if message != "the API refused the token: it was deleted, has expired, or is mistyped; create a new one on the portal" {
		t.Fatalf("message = %q", message)
	}
}

func TestExplain_OtherErrors_KeepTheCodeAndTitle(t *testing.T) {
	message := Explain(&Problem{Status: 422, Title: "Network still has VMs", Code: "NETWORK_NOT_EMPTY"})

	if message != "HTTP 422 NETWORK_NOT_EMPTY: Network still has VMs" {
		t.Fatalf("message = %q", message)
	}
}

func TestAnswerLost_OnlyForGatewayErrors(t *testing.T) {
	for _, status := range []int{502, 503, 504} {
		if !AnswerLost(status) {
			t.Fatalf("%d must count as a lost answer", status)
		}
	}
	for _, status := range []int{400, 409, 422, 500} {
		if AnswerLost(status) {
			t.Fatalf("%d is an answer, not a lost one", status)
		}
	}
}

func TestNewIdempotencyKey_IsNewEachTime(t *testing.T) {
	if NewIdempotencyKey() == NewIdempotencyKey() || len(NewIdempotencyKey()) != 32 {
		t.Fatal("keys must be 32 hex characters and differ")
	}
}
