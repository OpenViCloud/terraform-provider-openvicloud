package api

import (
	"errors"
	"net/http"
	"slices"
	"time"
)

const createAttempts = 3

// SendCreate sends one create (http-contract: a create needs an Idempotency-Key) and returns the new id. Every send
// carries the same key, so a create whose answer was lost (transport error, 502/503/504) is sent again without making
// a second resource; any other answer ends it.
func SendCreate(sleep func(time.Duration), send func(key string) (id *string, response *http.Response, body []byte, err error)) (string, error) {
	key := NewIdempotencyKey()
	var lastErr error
	for attempt := 1; attempt <= createAttempts; attempt++ {
		if attempt > 1 {
			sleep(2 * time.Second)
		}
		id, response, body, err := send(key)
		if err != nil {
			lastErr = err
			continue
		}
		if id != nil {
			return *id, nil
		}
		lastErr = errors.New(Explain(ProblemOf(response, body)))
		if !AnswerLost(response.StatusCode) {
			return "", lastErr
		}
	}
	return "", lastErr
}

// SendRepeatable sends a request the API answers the same way when it is repeated (attach, detach), so a request
// whose answer was lost is sent again. It returns the status when it is one of want; any other answer ends it with
// the API's reason.
func SendRepeatable(sleep func(time.Duration), send func() (response *http.Response, body []byte, err error), want ...int) (int, error) {
	var lastErr error
	for attempt := 1; attempt <= createAttempts; attempt++ {
		if attempt > 1 {
			sleep(2 * time.Second)
		}
		response, body, err := send()
		if err != nil {
			lastErr = err
			continue
		}
		if slices.Contains(want, response.StatusCode) {
			return response.StatusCode, nil
		}
		lastErr = errors.New(Explain(ProblemOf(response, body)))
		if !AnswerLost(response.StatusCode) {
			return 0, lastErr
		}
	}
	return 0, lastErr
}
