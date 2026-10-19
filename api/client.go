package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// Problem is the error body of every /v1 route: { status, title, code }.
type Problem struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Code   string `json:"code"`
}

func (p *Problem) Error() string {
	if p.Code == "" {
		return fmt.Sprintf("HTTP %d: %s", p.Status, p.Title)
	}
	return fmt.Sprintf("HTTP %d %s: %s", p.Status, p.Code, p.Title)
}

// ProblemOf reads the error body of a non-2xx answer. A body that is not a problem (a proxy's HTML page, an empty
// 401) still yields the HTTP status.
func ProblemOf(response *http.Response, body []byte) *Problem {
	var problem Problem
	err := json.Unmarshal(body, &problem)
	if err != nil {
		problem = Problem{}
	}
	if problem.Status == 0 {
		problem.Status = response.StatusCode
	}
	if problem.Title == "" {
		problem.Title = http.StatusText(response.StatusCode)
	}
	return &problem
}

// New is a client of the API at server that sends the customer's API token and the provider version.
func New(server string, token string, version string, httpClient *http.Client) (*ClientWithResponses, error) {
	addHeaders := func(ctx context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("User-Agent", "terraform-provider-openvicloud/"+version)
		return nil
	}
	return NewClientWithResponses(server, WithHTTPClient(httpClient), WithRequestEditorFn(addHeaders))
}

// Explain is the sentence a customer reads for a refused request: a 401 means the token itself.
func Explain(problem *Problem) string {
	if problem.Status == http.StatusUnauthorized {
		return "the API refused the token: it was deleted, has expired, or is mistyped; create a new one on the portal"
	}
	return problem.Error()
}

// NewIdempotencyKey is the key of one create: every send of that create carries it, so a create whose answer was
// lost and is sent again does not make a second resource.
func NewIdempotencyKey() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// AnswerLost: a gateway in front of the API failed, so the request may or may not have run; sending it again with
// the same key is safe.
func AnswerLost(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}
