package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func TestSendCreate_AnswerLost_SendsAgainWithTheSameKey(t *testing.T) {
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if len(keys) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"networkId":"9a1d0000-0000-4000-8000-000000000001"}`))
	}))
	defer server.Close()
	client, _ := api.New(server.URL, "ovc_x", "test", server.Client())

	id, err := sendCreate(context.Background(), client, api.CreateNetworkCommand{Name: "web", ZoneId: "local-mac"}, func(time.Duration) {})

	if err != nil || id != "9a1d0000-0000-4000-8000-000000000001" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("keys sent = %v", keys)
	}
}

func TestSendCreate_Refused_DoesNotSendAgain(t *testing.T) {
	sends := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"status":409,"title":"Network name taken","code":"NETWORK_NAME_TAKEN"}`))
	}))
	defer server.Close()
	client, _ := api.New(server.URL, "ovc_x", "test", server.Client())

	_, err := sendCreate(context.Background(), client, api.CreateNetworkCommand{Name: "web", ZoneId: "local-mac"}, func(time.Duration) {})

	if err == nil || sends != 1 || err.Error() != "HTTP 409 NETWORK_NAME_TAKEN: Network name taken" {
		t.Fatalf("sends = %d, err = %v", sends, err)
	}
}
