package test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chihsuanwu/tdxproxy/tdxproxy"
)

func TestGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tdxproxy.URL_AUTH {
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

	resp, err := proxy.Get("some_endpoint", nil, nil, 2*time.Second)

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
		if r.URL.Path == tdxproxy.URL_AUTH {
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

	resp, err := proxy.Get("some_endpoint", nil, nil, 2*time.Second)

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
		if r.URL.Path == tdxproxy.URL_AUTH {
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

	resp, err := proxy.Get("some_endpoint", nil, nil, 2*time.Second)

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
		if r.URL.Path == tdxproxy.URL_AUTH {
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

	resp, err := proxy.Get("some_endpoint", nil, nil, 2*time.Second)

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

	resp, err := proxy.Get("some_endpoint", nil, nil, 2*time.Second)

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("Expected nil response, got %v", resp)
	}
}
