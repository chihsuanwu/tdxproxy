# TDX Proxy

A Go client library for Taiwan TDX (Transport Data eXchange) platform API.

[![Go Version](https://img.shields.io/github/go-mod/go-version/chihsuanwu/tdxproxy)](https://github.com/chihsuanwu/tdxproxy)
[![Test](https://github.com/chihsuanwu/tdxproxy/actions/workflows/test.yml/badge.svg)](https://github.com/chihsuanwu/tdxproxy/actions/workflows/test.yml)

## Features

- Simple and intuitive API for accessing TDX platform
- Support for both authenticated and non-authenticated requests
- Automatic token refresh and retry mechanism
- Rate limiting handling with automatic retry
- Structured logging support

## Installation

```bash
go get github.com/chihsuanwu/tdxproxy
```

## Quick Start

### No Authentication (20 requests per day)

```go
package main

import (
	"io"
	"log"

	"github.com/chihsuanwu/tdxproxy"
)

func main() {
	// Create a proxy without authentication
	proxy := tdxproxy.NewNoAuthProxy(nil)

	// Make a request
	resp, err := proxy.Get("v2/Bus/Alert/City/Taichung", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Println(string(body))
}
```

### With Authentication (Unlimited requests)

```go
package main

import (
	"io"
	"log"

	"github.com/chihsuanwu/tdxproxy"
)

func main() {
	// Create a proxy with app credentials
	proxy := tdxproxy.NewProxy("your-app-id", "your-app-key", nil)

	// Make a request
	resp, err := proxy.Get("v2/Bus/Alert/City/Taichung", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Println(string(body))
}
```

### Using Credential File

Create a JSON file with your credentials:

```json
{
  "app_id": "your-app-id",
  "app_key": "your-app-key"
}
```

Then load it:

```go
proxy, err := tdxproxy.NewProxyFromCredentialFile("credentials.json", nil)
if err != nil {
    log.Fatal(err)
}
```

Or use environment variable:

```bash
export TDX_CREDENTIALS_FILE=/path/to/credentials.json
```

```go
proxy, err := tdxproxy.NewProxyFromCredentialFile("", nil)
if err != nil {
    log.Fatal(err)
}
```

## Advanced Usage

### Custom Configuration

```go
import (
    "log/slog"
    "time"
)

// Create custom logger
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Create proxy with custom settings
proxy := tdxproxy.NewProxy("app-id", "app-key", logger)
proxy.SetTimeout(30 * time.Second)
proxy.SetHost("https://custom-host.example.com")
proxy.SetBaseURL("/api/v3/")
```

## API Documentation

### Main Functions

- `NewProxy(appID, appKey string, logger *slog.Logger) *TDXProxy` - Create authenticated proxy
- `NewNoAuthProxy(logger *slog.Logger) *TDXProxy` - Create non-authenticated proxy
- `NewProxyFromCredentialFile(fileName string, logger *slog.Logger) (*TDXProxy, error)` - Create proxy from credential file

### Methods

- `Get(url string, params map[string]string, headers map[string]string) (*http.Response, error)` - Send GET request
- `SetBaseURL(url string)` - Set custom base URL path
- `SetHost(url string)` - Set custom host URL
- `SetTimeout(timeout time.Duration)` - Set request timeout

### Constants

- `DefaultHost` - Default TDX platform host URL

## Error Handling

The library automatically handles common scenarios:

- **401 Unauthorized**: Automatically refreshes the authentication token and retries
- **429 Too Many Requests**: Waits and retries (up to 3 attempts)
- **Network errors**: Returns detailed error messages

## Examples

See the [examples](examples/) directory for complete working examples.

## License

This project is licensed under the MIT License — see the LICENSE file for details.

## Resources

- [TDX Platform Official Website](https://tdx.transportdata.tw/)
- [TDX API Documentation](https://tdx.transportdata.tw/api-service/swagger)
- [GoDoc](https://pkg.go.dev/github.com/chihsuanwu/tdxproxy)
