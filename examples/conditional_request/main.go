package main

import (
	"fmt"
	"io"
	"log"

	"github.com/chihsuanwu/tdxproxy"
)

func main() {
	// Create a proxy with authentication
	proxy := tdxproxy.NewNoAuthProxy(nil)

	// First request - will receive data and store Last-Modified header
	fmt.Println("Making first request...")
	resp1, err := proxy.Get("v2/Bus/Alert/City/Taichung", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("First request status: %d\n", resp1.StatusCode)
	if lastModified := resp1.Header.Get("Last-Modified"); lastModified != "" {
		fmt.Printf("Last-Modified header received: %s\n", lastModified)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	fmt.Printf("Response length: %d bytes\n\n", len(body1))

	// Second request to the same endpoint
	// If data hasn't changed, server will return 304 Not Modified
	fmt.Println("Making second request to the same endpoint...")
	resp2, err := proxy.Get("v2/Bus/Alert/City/Taichung", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Second request status: %d\n", resp2.StatusCode)
	if resp2.StatusCode == 304 {
		fmt.Println("Server returned 304 Not Modified - data hasn't changed!")
	}
	resp2.Body.Close()
}
