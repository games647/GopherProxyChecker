package main

import (
	"log"
	"net/http"
	"h12.me/socks"
	"time"
	"os"
	"bufio"
)

const TIMEOUT = time.Duration(5 * time.Second)

func main() {
	input, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	defer input.Close()

	working := make([]string, 0)

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		proxyLine := scanner.Text()
		log.Println("Testing ", proxyLine)

		if testProxy(proxyLine) {
			working = append(working, proxyLine)
		}
	}

	if err = scanner.Err(); err != nil {
		log.Fatal(err)
	}

	log.Println("Working", working)
	if _, err := os.Stat(os.Args[2]); os.IsNotExist(err) {
		// path doesn't exist does not exist
		os.Create(os.Args[2])
	}

	output, err := os.OpenFile(os.Args[2], os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}

	for _, proxy := range working {
		_, err := output.WriteString(proxy + "\n")
		if err != nil {
			log.Fatal(err)
		}
	}

	output.Sync()
	defer output.Close()
}

func testProxy(line string) bool {
	dialSocksProxy := socks.DialSocksProxy(socks.SOCKS5, line)
	tr := &http.Transport{Dial: dialSocksProxy}

	httpClient := &http.Client{Transport: tr, Timeout: TIMEOUT}
	resp, err := httpClient.Get("http://www.google.com")
	if err != nil {
		log.Println(err)
		return false
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Println(resp.StatusCode)
		return false
	}

	log.Println("Working proxy (SOCKS5)", line)
	return true
}