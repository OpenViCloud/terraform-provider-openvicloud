package firewallrule

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func TestSendCreate_AnswerLost_SendsAgainWithTheSameKey(t *testing.T) {
	var keys, bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if len(keys) == 1 {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ruleId":"7e1d0000-0000-4000-8000-000000000001"}`))
	}))
	defer server.Close()
	client, _ := api.New(server.URL, "ovc_x", "test", server.Client())

	id, err := sendCreate(context.Background(), client, "n-1", ruleBody{Direction: "Ingress", Protocol: "TCP",
		StartPort: ptr(int32(22)), EndPort: ptr(int32(22)), Cidr: "0.0.0.0/0"}, func(time.Duration) {})

	if err != nil || id != "7e1d0000-0000-4000-8000-000000000001" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] || bodies[0] != bodies[1] {
		t.Fatalf("keys sent = %v, bodies = %v", keys, bodies)
	}
}

func TestSendCreate_Icmp_SendsNoPorts(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ruleId":"r-1"}`))
	}))
	defer server.Close()
	client, _ := api.New(server.URL, "ovc_x", "test", server.Client())

	_, err := sendCreate(context.Background(), client, "n-1", ruleBody{Direction: "Ingress", Protocol: "ICMP", Cidr: "0.0.0.0/0"}, func(time.Duration) {})

	if err != nil || body != `{"direction":"Ingress","protocol":"ICMP","cidr":"0.0.0.0/0"}` {
		t.Fatalf("body = %s, err = %v", body, err)
	}
}
