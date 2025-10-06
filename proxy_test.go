package tdxproxy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chihsuanwu/tdxproxy"
)

const authPath = "/auth/realms/TDXConnect/protocol/openid-connect/token"

func TestGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token": "token", "expires_in": 3600}`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":"success"}`))
		}
	}))
	defer server.Close()

	proxy := tdxproxy.NewProxy("appID", "appKey", nil)
	proxy.SetHost(server.URL)

	resp, err := proxy.Get("some_endpoint", nil, nil)

	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", resp.StatusCode)
	}
}

func TestGet_Unauthorized_RefreshToken(t *testing.T) {
	tokenRequestCount := 0
	apiRequestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authPath {
			tokenRequestCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token": "token", "expires_in": 3600}`))
		} else {
			apiRequestCount++
			if apiRequestCount == 1 {
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data":"success"}`))
			}
		}
	}))
	defer server.Close()

	proxy := tdxproxy.NewProxy("appID", "appKey", nil)
	proxy.SetHost(server.URL)

	resp, err := proxy.Get("some_endpoint", nil, nil)

	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", resp.StatusCode)
	}

	if tokenRequestCount != 2 {
		t.Fatalf("Expected token request count 2, got %d", tokenRequestCount)
	}

	if apiRequestCount != 2 {
		t.Fatalf("Expected api request count 2, got %d", apiRequestCount)
	}
}

func TestGet_RateLimit_Retry(t *testing.T) {
	apiRequestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token": "token", "expires_in": 3600}`))
		} else {
			apiRequestCount++
			if apiRequestCount <= 2 {
				w.WriteHeader(http.StatusTooManyRequests)
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data":"success"}`))
			}
		}
	}))
	defer server.Close()

	proxy := tdxproxy.NewProxy("appID", "appKey", nil)
	proxy.SetHost(server.URL)

	resp, err := proxy.Get("some_endpoint", nil, nil)

	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", resp.StatusCode)
	}

	if apiRequestCount != 3 {
		t.Fatalf("Expected api request count 3, got %d", apiRequestCount)
	}
}

func TestGet_MaxRetriesReached(t *testing.T) {
	apiRequestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token": "token", "expires_in": 3600}`))
		} else {
			apiRequestCount++
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer server.Close()

	proxy := tdxproxy.NewProxy("appID", "appKey", nil)
	proxy.SetHost(server.URL)

	resp, err := proxy.Get("some_endpoint", nil, nil)

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("Expected nil response, got %v", resp)
	}

	if apiRequestCount != 3 {
		t.Fatalf("Expected api request count 3, got %d", apiRequestCount)
	}
}

func TestGet_NetworkError(t *testing.T) {
	proxy := tdxproxy.NewProxy("appID", "appKey", nil)
	proxy.SetHost("http://localhost:12345")

	resp, err := proxy.Get("some_endpoint", nil, nil)

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("Expected nil response, got %v", resp)
	}
}

func TestGet_ConditionalRequest_LastModified(t *testing.T) {
	requestCount := 0
	lastModifiedTime := "Mon, 02 Jan 2006 15:04:05 GMT"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token": "token", "expires_in": 3600}`))
		} else {
			requestCount++
			if requestCount == 1 {
				// First request: return data with Last-Modified header
				w.Header().Set("Last-Modified", lastModifiedTime)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data":"success"}`))
			} else {
				// Second request: check for If-Modified-Since header
				ifModifiedSince := r.Header.Get("If-Modified-Since")
				if ifModifiedSince == lastModifiedTime {
					// Return 304 Not Modified
					w.WriteHeader(http.StatusNotModified)
				} else {
					t.Errorf("Expected If-Modified-Since header to be %s, got %s", lastModifiedTime, ifModifiedSince)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"data":"success"}`))
				}
			}
		}
	}))
	defer server.Close()

	proxy := tdxproxy.NewProxy("appID", "appKey", nil)
	proxy.SetHost(server.URL)

	// First request
	resp1, err := proxy.Get("some_endpoint", nil, nil)
	if err != nil {
		t.Fatalf("Error on first request: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200 on first request, got %d", resp1.StatusCode)
	}
	resp1.Body.Close()

	// Second request - should send If-Modified-Since and receive 304
	resp2, err := proxy.Get("some_endpoint", nil, nil)
	if err != nil {
		t.Fatalf("Error on second request: %v", err)
	}
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("Expected status code 304 on second request, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	if requestCount != 2 {
		t.Fatalf("Expected 2 API requests, got %d", requestCount)
	}
}
