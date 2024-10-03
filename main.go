package main

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/chihsuanwu/tdxproxy/tdxproxy"
)

func main() {
	logger := log.New(os.Stdout, "TDX Proxy: ", log.LstdFlags)
	proxy := tdxproxy.NewTDXProxyNoAuth(logger)

	resp, err := proxy.Get("v2/Bus/Alert/City/Taichung", nil, nil, 10*time.Second)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}
	log.Println(string(body))
}
