package api

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func answer(status int, id *string) func(string) (*string, *http.Response, []byte, error) {
	return func(key string) (*string, *http.Response, []byte, error) {
		return id, &http.Response{StatusCode: status}, nil, nil
	}
}

func TestSendCreate_LostThenAnswered_UsesOneKeyForEverySend(t *testing.T) {
	var keys []string
	id := "new-id"
	statuses := []int{502, 504, 202}
	got, err := SendCreate(func(time.Duration) {}, func(key string) (*string, *http.Response, []byte, error) {
		keys = append(keys, key)
		status := statuses[len(keys)-1]
		if status == 202 {
			return &id, &http.Response{StatusCode: status}, nil, nil
		}
		return nil, &http.Response{StatusCode: status}, nil, nil
	})

	if err != nil || got != "new-id" {
		t.Fatalf("id = %q, err = %v", got, err)
	}
	if len(keys) != 3 || keys[0] == "" || keys[0] != keys[1] || keys[1] != keys[2] {
		t.Fatalf("keys = %v", keys)
	}
}

func TestSendCreate_TransportErrorEveryTime_GivesUpAfterThreeSends(t *testing.T) {
	sends := 0
	_, err := SendCreate(func(time.Duration) {}, func(key string) (*string, *http.Response, []byte, error) {
		sends++
		return nil, nil, nil, errors.New("connection reset")
	})

	if err == nil || sends != 3 {
		t.Fatalf("sends = %d, err = %v", sends, err)
	}
}

func TestSendCreate_Refused_StopsAtTheFirstAnswer(t *testing.T) {
	sends := 0
	_, err := SendCreate(func(time.Duration) {}, func(key string) (*string, *http.Response, []byte, error) {
		sends++
		return nil, &http.Response{StatusCode: 422}, []byte(`{"status":422,"title":"Quota","code":"VM_QUOTA_EXCEEDED"}`), nil
	})

	if sends != 1 || err == nil || err.Error() != "HTTP 422 VM_QUOTA_EXCEEDED: Quota" {
		t.Fatalf("sends = %d, err = %v", sends, err)
	}
}

func TestSendRepeatable_LostThenAccepted_IsDone(t *testing.T) {
	sends := 0
	statuses := []int{503, 202}
	status, err := SendRepeatable(func(time.Duration) {}, func() (*http.Response, []byte, error) {
		sends++
		return &http.Response{StatusCode: statuses[sends-1]}, nil, nil
	}, http.StatusAccepted, http.StatusNotFound)

	if err != nil || sends != 2 || status != http.StatusAccepted {
		t.Fatalf("sends = %d, err = %v", sends, err)
	}
}

func TestSendRepeatable_Refused_StopsAtTheFirstAnswer(t *testing.T) {
	sends := 0
	_, err := SendRepeatable(func(time.Duration) {}, func() (*http.Response, []byte, error) {
		sends++
		return &http.Response{StatusCode: 422}, nil, nil
	}, http.StatusAccepted)

	if err == nil || sends != 1 {
		t.Fatalf("sends = %d, err = %v", sends, err)
	}
}
