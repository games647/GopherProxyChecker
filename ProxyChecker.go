package main

import (
	"log"
	"net/http"
	"h12.me/socks"
	"time"
	"os"
	"bufio"
	"errors"
	"net/url"
)

const TIMEOUT = time.Duration(30 * time.Second)
var REDIRECT_ERROR = errors.New("Host redirected to different target")

func main() {
	log.Println("Loading input")

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

		if testProxy(proxyLine, true) {
			working = append(working, proxyLine)
		} else if testProxy(proxyLine, false) {
			log.Println("Working SOCKS4")
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

	defer output.Close()

	writer := bufio.NewWriter(output)

	for _, proxy := range working {
		_, err := writer.WriteString(proxy + "\n")
		if err != nil {
			log.Fatal(err)
		}
	}

	writer.Flush()
	output.Sync()
}

func testProxy(line string, socks5 bool) bool {
	httpClient := &http.Client{
		Transport: createSocksProxy(socks5, line),
		Timeout: TIMEOUT,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return REDIRECT_ERROR
		},
	}

	resp, err := httpClient.Get("http://www.google.com")
	if err != nil {
		// test if we got the custom error
		if urlError, ok := err.(*url.Error); ok && urlError.Err == REDIRECT_ERROR {
			log.Println("Working proxy on redirect (SOCKS5)", line)
			return true
		}

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

func createSocksProxy(socks5 bool, proxy string) *http.Transport {
	if socks5 {
		dialSocksProxy := socks.DialSocksProxy(socks.SOCKS5, proxy)
		tr := &http.Transport{Dial: dialSocksProxy}
		return tr;
	}

	dialSocksProxy := socks.DialSocksProxy(socks.SOCKS4, proxy)
	tr := &http.Transport{Dial: dialSocksProxy}
	return tr;
}