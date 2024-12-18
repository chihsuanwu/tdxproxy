package tdxproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	TDX_URL_BASIC = "https://tdx.transportdata.tw/api/basic/"
	authURL       = "https://tdx.transportdata.tw/auth/realms/TDXConnect/protocol/openid-connect/token"
)

// TDXProxy simplifies the interface process with the TDX platform.
// You can directly call the TDX platform's API as long as
// the Client ID and Secret Key are provided.
type TDXProxy struct {
	appID       string
	appKey      string
	authToken   string
	baseUrl     string
	expiredTime int64
	logger      *slog.Logger
}

func NewTDXProxy(appID, appKey string, logger *slog.Logger) *TDXProxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &TDXProxy{
		appID:       appID,
		appKey:      appKey,
		baseUrl:     TDX_URL_BASIC,
		authToken:   "",
		expiredTime: time.Now().Unix(),
		logger:      logger,
	}
}

// NewTDXProxyFromCredentialFile creates a new TDXProxy instance using credentials from a file.
// The file should be in JSON format and contain the following fields:
//
//	{
//	  "app_id": "app_id",
//	  "app_key": "app_key"
//	}
//
// The file path can be specified as the first argument, or the TDX_CREDENTIALS_FILE environment variable.
func NewTDXProxyFromCredentialFile(fileName string, logger *slog.Logger) (*TDXProxy, error) {
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

	return NewTDXProxy(credentials.AppID, credentials.AppKey, logger), nil
}

func NewTDXProxyNoAuth(logger *slog.Logger) *TDXProxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &TDXProxy{
		appID:   "",
		appKey:  "",
		baseUrl: TDX_URL_BASIC,
		logger:  logger,
	}
}

func (proxy *TDXProxy) Get(url string, params map[string]string, headers map[string]string, timeout time.Duration) (*http.Response, error) {
	if params == nil {
		params = map[string]string{"$format": "JSON"}
	}
	return proxy.requestWithRetry(url, params, headers, timeout, 0)
}

func (proxy *TDXProxy) SetBaseURL(url string) {
	if url == "" {
		proxy.logger.Warn("Empty base URL provided")
		return
	}

	proxy.baseUrl = url
}

func (proxy *TDXProxy) requestWithRetry(url string, params, headers map[string]string, timeout time.Duration, retryCount int) (*http.Response, error) {
	if retryCount > 2 {
		return nil, fmt.Errorf("max retry attempts reached for %s", url)
	}

	fullURL := proxy.buildFullURL(url, params)
	reqHeaders, err := proxy.buildAuthHeaders(timeout)
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

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if err := proxy.handleResponse(resp, url, params, headers, timeout, retryCount); err != nil {
		return nil, err
	}
	return resp, nil
}

func (proxy *TDXProxy) handleResponse(resp *http.Response, url string, params, headers map[string]string, timeout time.Duration, retryCount int) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotModified:
		proxy.logger.Info("Successful request", slog.String("url", url), slog.Int("status", resp.StatusCode))
		return nil
	case http.StatusUnauthorized:
		proxy.logger.Warn("Unauthorized, refreshing token...", slog.String("url", url))
		if err := proxy.updateAuth(timeout); err != nil {
			return fmt.Errorf("failed to refresh auth token: %w", err)
		}
		proxy.logger.Info("Retrying request after refreshing token")
		_, err := proxy.requestWithRetry(url, params, headers, timeout, retryCount+1)
		return err
	case http.StatusTooManyRequests:
		proxy.logger.Warn("Rate limit reached, retrying...", slog.String("url", url))
		time.Sleep(1 * time.Second)
		_, err := proxy.requestWithRetry(url, params, headers, timeout, retryCount+1)
		return err
	default:
		proxy.logger.Error("Unexpected status code", slog.String("url", url), slog.Int("status", resp.StatusCode))
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// buildFullURL constructs the full API URL with query parameters.
func (proxy *TDXProxy) buildFullURL(url string, params map[string]string) string {
	var builder strings.Builder
	builder.WriteString(proxy.baseUrl)
	builder.WriteString(url)
	builder.WriteString("?")

	for k, v := range params {
		builder.WriteString(fmt.Sprintf("%s=%s&", k, v))
	}
	return strings.TrimSuffix(builder.String(), "&")
}

// buildAuthHeaders constructs headers including authorization if applicable.
func (proxy *TDXProxy) buildAuthHeaders(timeout time.Duration) (map[string]string, error) {
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.122 Safari/537.36",
	}

	if proxy.appID == "" || proxy.appKey == "" {
		return headers, nil
	}

	if proxy.authToken == "" || time.Now().Unix() > proxy.expiredTime {
		if err := proxy.updateAuth(timeout); err != nil {
			proxy.logger.Error("Failed to update auth token", slog.String("error", err.Error()))
			return nil, err
		}
	}
	headers["Authorization"] = "Bearer " + proxy.authToken
	return headers, nil
}

// updateAuth fetches a new authentication token.
func (proxy *TDXProxy) updateAuth(timeout time.Duration) error {
	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s", proxy.appID, proxy.appKey)
	req, err := http.NewRequest("POST", authURL, bytes.NewBufferString(data))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: timeout}
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
