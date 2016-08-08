package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"h12.me/socks"
)

func main() {
	dialSocksProxy := socks.DialSocksProxy(socks.SOCKS5, "103.232.150.148:48111")
	tr := &http.Transport{Dial: dialSocksProxy}
	httpClient := &http.Client{Transport: tr}
	resp, err := httpClient.Get("http://www.google.com")
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatal(resp.StatusCode)
	}

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(buf))
}