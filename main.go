package main

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/chihsuanwu/tdxproxy/tdxproxy"
)

func main() {
	proxy := tdxproxy.NewTDXProxyNoAuth(nil)

	url := "v2/Bus/Alert/City/Taichung"

	resp, err := proxy.Get(url, nil, nil, 10*time.Second)
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
