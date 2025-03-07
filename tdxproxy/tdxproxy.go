package tdxproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	TDX_HOST  = "https://tdx.transportdata.tw"
	URL_BASIC = "/api/basic/"
	URL_AUTH  = "/auth/realms/TDXConnect/protocol/openid-connect/token"
)

// Proxy simplifies the interface process with the TDX platform.
// You can directly call the TDX platform's API as long as
// the Client ID and Secret Key are provided.
type Proxy interface {
	Get(url string, params map[string]string, headers map[string]string) (*http.Response, error)
	SetBaseURL(url string)
	SetHost(url string)
	SetTimeout(timeout time.Duration)
}

type TDXProxy struct {
	appID       string
	appKey      string
	authToken   string
	host        string
	baseUrl     string
	expiredTime int64
	timeout     time.Duration
	logger      *slog.Logger
}

// NewProxy creates a new TDXProxy instance.
// The appID and appKey are required for authentication.
func NewProxy(appID, appKey string, logger *slog.Logger) *TDXProxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &TDXProxy{
		appID:       appID,
		appKey:      appKey,
		authToken:   "",
		baseUrl:     URL_BASIC,
		host:        TDX_HOST,
		expiredTime: time.Now().Unix(),
		timeout:     10 * time.Second,
		logger:      logger,
	}
}

// NewProxyFromCredentialFile creates a new TDXProxy instance using credentials from a file.
// The file should be in JSON format and contain the following fields:
//
//	{
//	  "app_id": "app_id",
//	  "app_key": "app_key"
//	}
//
// The file path can be specified as the first argument, or the TDX_CREDENTIALS_FILE environment variable.
func NewProxyFromCredentialFile(fileName string, logger *slog.Logger) (*TDXProxy, error) {
	if fileName == "" {
		fileName = os.Getenv("TDX_CREDENTIALS_FILE")
	}
	if fileName == "" {
		return nil, errors.New("no credential file specified and TDX_CREDENTIALS_FILE environment variable is not set")
	}

	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	credentials := struct {
		AppID  string `json:"app_id"`
		AppKey string `json:"app_key"`
	}{}
	if err := json.NewDecoder(file).Decode(&credentials); err != nil {
		return nil, err
	}

	return NewProxy(credentials.AppID, credentials.AppKey, logger), nil
}

// NewNoAuthProxy creates a new TDXProxy instance without authentication.
// With this instance, you can make up to 20 requests per day.
func NewNoAuthProxy(logger *slog.Logger) *TDXProxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &TDXProxy{
		appID:   "",
		appKey:  "",
		host:    TDX_HOST,
		baseUrl: URL_BASIC,
		timeout: 10 * time.Second,
		logger:  logger,
	}
}

// Get sends a GET request to the TDX platform.
// If the request fails due to an expired token, it will attempt to refresh the token and retry.
// If the request fails due to rate limiting, it will retry after a short delay. The maximum number of retries is 3.
func (proxy *TDXProxy) Get(url string, params map[string]string, headers map[string]string) (*http.Response, error) {
	return proxy.requestWithRetry(url, params, headers, 0)
}

// SetBaseURL sets the base URL for the TDX platform.
// Default is URL_BASIC
func (proxy *TDXProxy) SetBaseURL(url string) {
	if url == "" {
		proxy.logger.Warn("Empty base URL provided")
		return
	}

	proxy.baseUrl = url
}

// SetHost sets the host URL for the TDX platform.
// Default is TDX_HOST
func (proxy *TDXProxy) SetHost(url string) {
	if url == "" {
		proxy.logger.Warn("Empty host URL provided")
		return
	}

	proxy.host = url
}

// SetTimeout sets the timeout for requests to the TDX platform.
func (proxy *TDXProxy) SetTimeout(timeout time.Duration) {
	if timeout <= 0 {
		proxy.logger.Warn("Invalid timeout provided")
		return
	}

	proxy.timeout = timeout
}

func (proxy *TDXProxy) requestWithRetry(url string, params, headers map[string]string, retryCount int) (*http.Response, error) {
	if retryCount > 2 {
		return nil, fmt.Errorf("max retry attempts reached for %s", url)
	}

	fullURL := proxy.buildFullURL(url, params)
	reqHeaders, err := proxy.buildAuthHeaders()
	if err != nil {
		return nil, fmt.Errorf("failed to build auth headers: %w", err)
	}
	for k, v := range headers {
		reqHeaders[k] = v
	}

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: proxy.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	resp, err = proxy.handleResponse(resp, url, params, headers, retryCount)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (proxy *TDXProxy) handleResponse(resp *http.Response, url string, params, headers map[string]string, retryCount int) (*http.Response, error) {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotModified:
		proxy.logger.Info("Successful request", slog.String("url", url), slog.Int("status", resp.StatusCode))
		return resp, nil
	case http.StatusUnauthorized:
		proxy.logger.Warn("Unauthorized, refreshing token...", slog.String("url", url))
		resp.Body.Close()
		if err := proxy.updateAuth(); err != nil {
			return nil, fmt.Errorf("failed to refresh auth token: %w", err)
		}
		proxy.logger.Info("Retrying request after refreshing token")
		resp, err := proxy.requestWithRetry(url, params, headers, retryCount+1)
		return resp, err
	case http.StatusTooManyRequests:
		proxy.logger.Warn("Rate limit reached, retrying...", slog.String("url", url))
		resp.Body.Close()
		time.Sleep(1 * time.Second)
		resp, err := proxy.requestWithRetry(url, params, headers, retryCount+1)
		return resp, err
	default:
		proxy.logger.Error("Unexpected status code", slog.String("url", url), slog.Int("status", resp.StatusCode))
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// buildFullURL constructs the full API URL with query parameters.
func (proxy *TDXProxy) buildFullURL(queryUrl string, params map[string]string) string {
	var builder strings.Builder
	builder.WriteString(proxy.host)
	builder.WriteString(proxy.baseUrl)
	builder.WriteString(queryUrl)
	builder.WriteString("?")

	for k, v := range params {
		builder.WriteString(fmt.Sprintf("%s=%s&", url.QueryEscape(k), url.QueryEscape(v)))
	}
	return strings.TrimSuffix(builder.String(), "&")
}

// buildAuthHeaders constructs headers including authorization if applicable.
func (proxy *TDXProxy) buildAuthHeaders() (map[string]string, error) {
	if proxy.appID == "" || proxy.appKey == "" { // no auth, return browser headers for 20 requests per day
		headers := map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.122 Safari/537.36",
		}
		return headers, nil
	}

	if proxy.authToken == "" || time.Now().Unix() > proxy.expiredTime {
		if err := proxy.updateAuth(); err != nil {
			proxy.logger.Error("Failed to update auth token", slog.String("error", err.Error()))
			return nil, err
		}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + proxy.authToken,
	}
	return headers, nil
}

// updateAuth fetches a new authentication token.
func (proxy *TDXProxy) updateAuth() error {
	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s", proxy.appID, proxy.appKey)
	req, err := http.NewRequest("POST", proxy.host+URL_AUTH, bytes.NewBufferString(data))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: proxy.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth request returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read auth response: %w", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	token, ok := response["access_token"].(string)
	if !ok {
		return errors.New("auth response missing access_token")
	}
	expiresIn, ok := response["expires_in"].(float64)
	if !ok {
		return errors.New("auth response missing expires_in")
	}

	proxy.authToken = token
	proxy.expiredTime = time.Now().Unix() + int64(expiresIn) - 60
	return nil
}

var _ Proxy = (*TDXProxy)(nil)
