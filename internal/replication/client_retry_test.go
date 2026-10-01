package replication

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataBackpressureRetry(t *testing.T) {
	for _, test := range []struct {
		name                   string
		failures, status, want int
		retryable              bool
	}{
		{"transient", 2, 429, 3, true}, {"bounded", 10, 429, MaxRetryAttempts, true}, {"unauthorized", 10, 401, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls <= test.failures {
					w.WriteHeader(test.status)
					_ = json.NewEncoder(w).Encode(ErrorResponse{Code: "RETRY_EXHAUSTED", Retryable: test.retryable})
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, http: server.Client()}
			err := client.postJSON(context.Background(), "/test", struct{}{}, &struct{}{})
			if (err == nil) != (test.failures < MaxRetryAttempts) || calls != test.want {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
