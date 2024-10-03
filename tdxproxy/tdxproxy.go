package tdxproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	TDX_URL_BASE = "https://tdx.transportdata.tw/api/basic/"
	AUTH_URL     = "https://tdx.transportdata.tw/auth/realms/TDXConnect/protocol/openid-connect/token"
)

// TDXProxy simplifies the interface process with the TDX platform.
// You can directly call the TDX platform's API as long as
// the Client ID and Secret Key are provided.
type TDXProxy struct {
	appID       string
	appKey      string
	authToken   string
	expiredTime int64
	logger      *log.Logger
}

func NewTDXProxy(appID, appKey string, logger *log.Logger) *TDXProxy {
	return &TDXProxy{
		appID:       appID,
		appKey:      appKey,
		authToken:   "",
		expiredTime: time.Now().Unix(),
		logger:      logger,
	}
}

func NewTDXProxyFromCredentialFile(fileName string, logger *log.Logger) (*TDXProxy, error) {
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

func NewTDXProxyNoAuth(logger *log.Logger) *TDXProxy {
	return &TDXProxy{
		appID:  "",
		appKey: "",
		logger: logger,
	}
}

func (proxy *TDXProxy) Get(url string, params map[string]string, headers map[string]string, timeout time.Duration) (*http.Response, error) {
	if params == nil {
		params = map[string]string{
			"$format": "JSON",
		}
	}

	return proxy.getAPI(url, TDX_URL_BASE, params, headers, timeout, 0)
}

func (proxy *TDXProxy) getAPI(url, urlBase string, params, headers map[string]string, timeout time.Duration, retryTimes int) (*http.Response, error) {
	requestHeaders := proxy.getAuthHeader(timeout)
	for k, v := range headers {
		requestHeaders[k] = v
	}

	// Add query parameters to the URL
	queryString := "?"
	for k, v := range params {
		queryString += k + "=" + v + "&"
	}
	fullURL := urlBase + url + strings.TrimRight(queryString, "&")

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range requestHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		proxy.logger.Printf("TDX Proxy get %s, status %d", url, resp.StatusCode)
		if retryTimes >= 2 {
			return resp, nil
		}
	} else {
		proxy.logger.Printf("TDX Proxy get %s, status %d", url, resp.StatusCode)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		if proxy.appID == "" || proxy.appKey == "" {
			proxy.logger.Println("Authentication required, please provide APP ID and KEY")
			return resp, nil
		}

		proxy.logger.Println("Fetch new token...")
		if err := proxy.updateAuth(timeout); err != nil {
			return nil, err
		}
		proxy.logger.Println("Retrying...")
		return proxy.getAPI(url, urlBase, params, headers, timeout, retryTimes+1)

	case http.StatusTooManyRequests:
		if proxy.appID == "" || proxy.appKey == "" {
			proxy.logger.Println("TDX API daily limit exceeded, please provide APP ID and KEY")
			return resp, nil
		}

		proxy.logger.Println("Waiting 1 sec...")
		time.Sleep(1 * time.Second)
		proxy.logger.Println("Retrying...")
		return proxy.getAPI(url, urlBase, params, headers, timeout, retryTimes+1)
	}

	return resp, nil
}

func (proxy *TDXProxy) getAuthHeader(timeout time.Duration) map[string]string {
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.122 Safari/537.36",
	}

	if proxy.appID == "" || proxy.appKey == "" {
		return headers
	}

	if proxy.authToken == "" || time.Now().Unix() > proxy.expiredTime {
		if err := proxy.updateAuth(timeout); err != nil {
			proxy.logger.Printf("Error updating auth: %v", err)
		}
	}

	headers["Authorization"] = "Bearer " + proxy.authToken
	return headers
}

func (proxy *TDXProxy) updateAuth(timeout time.Duration) error {
	data := "grant_type=client_credentials&client_id=" + proxy.appID + "&client_secret=" + proxy.appKey
	req, err := http.NewRequest("POST", AUTH_URL, bytes.NewBufferString(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("failed to fetch auth token")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}

	proxy.authToken = response["access_token"].(string)
	expiresIn := int64(response["expires_in"].(float64))
	proxy.expiredTime = time.Now().Unix() + expiresIn - 60

	return nil
}
