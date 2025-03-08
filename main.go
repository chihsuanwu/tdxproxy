package main

import (
	"github.com/chihsuanwu/tdxproxy/tdxproxy"
	"io"
	"log"
	"os"
)

func main() {
	proxy := tdxproxy.NewNoAuthProxy(nil)

	url := "v2/Bus/Alert/City/Taichung"

	resp, err := proxy.Get(url, nil, nil)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}
	log.Println(string(body))
	log.Println("headers:", resp.Header)

	// save to file
	os.WriteFile("response.json", body, 0644)
}
