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
	"sync"
	"sync/atomic"
	"github.com/oschwald/geoip2-golang"
	"net"
	"strings"
)

const TIMEOUT = time.Duration(5 * time.Second)
const WORKER_THREADS = 30
//downloadable at: https://dev.maxmind.com/geoip/geoip2/geolite2/
const GEO_IP_FILE = "GeoLite2-Country.mmdb"
const TEST_TARGET = "http://www.google.com"

var REDIRECT_ERROR = errors.New("Host redirected to different target")

func main() {
	log.Println("Loading input")

	input, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	defer input.Close()

	working := make([]string, 0)

	var db *geoip2.Reader
	if _, err := os.Stat(GEO_IP_FILE); err == nil {
		log.Println("GEO-IP File found")
		dbFile, err := geoip2.Open(GEO_IP_FILE)
		if err != nil {
			log.Fatal(err)
		}

		db = dbFile
	}

	defer func() {
		if db != nil {
			db.Close()
		}
	}()

	var readMutex = &sync.Mutex{}
	var writeMutex = &sync.Mutex{}

	var testIndex uint32 = 0

	var wg sync.WaitGroup

	scanner := bufio.NewScanner(input)
	for i := 0; i < WORKER_THREADS; i++ {
		wg.Add(1)
		go func() {
			for {
				readMutex.Lock()
				if !scanner.Scan() {
					readMutex.Unlock()
					break
				}

				proxyLine := scanner.Text()
				readMutex.Unlock()

				index := atomic.AddUint32(&testIndex, 1)

				countryIso := ""
				if db != nil {
					ip := net.ParseIP(strings.Split(proxyLine, ":")[0])
					country, err := db.Country(ip)

					if err == nil {
						countryIso = country.Country.IsoCode
					}
				}

				log.Println("Testing ", index, proxyLine)
				if works, time := testProxy(proxyLine, true); works {
					log.Println("Working SOCKS5", index, proxyLine, time, "ms", countryIso)

					writeMutex.Lock()
					working = append(working, proxyLine)
					writeMutex.Unlock()
				} else if works, time := testProxy(proxyLine, false); works {
					log.Println("Working SOCKS4", index, proxyLine, time, "ms", countryIso)

					writeMutex.Lock()
					working = append(working, proxyLine)
					writeMutex.Unlock()
				}
			}

			wg.Done()
		}()
	}

	wg.Wait()

	if err = scanner.Err(); err != nil {
		log.Fatal(err)
	}

	log.Println("Working", working)
	writeWorkingProxies(working)
}

func writeWorkingProxies(working []string) {
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
}

func testProxy(line string, socks5 bool) (bool, int64) {
	httpClient := &http.Client{
		Transport: createSocksProxy(socks5, line),
		Timeout: TIMEOUT,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return REDIRECT_ERROR
		},
	}

	start := time.Now()
	resp, err := httpClient.Get(TEST_TARGET)
	end := time.Now()
	responseTime := end.Sub(start).Nanoseconds() / 1000000
	if err != nil {
		if urlError, ok := err.(*url.Error); ok && urlError.Err == REDIRECT_ERROR {
			// test if we got the custom error
			log.Println("Redirect", line)
			return true, responseTime
		}

		log.Println(err)
		return false, 0
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Println(resp.StatusCode)
		return false, 0
	}

	return true, responseTime
}

func createSocksProxy(socks5 bool, proxy string) *http.Transport {
	var dialSocksProxy func(string, string) (net.Conn, error)
	if socks5 {
		dialSocksProxy = socks.DialSocksProxy(socks.SOCKS5, proxy)
	} else {
		dialSocksProxy = socks.DialSocksProxy(socks.SOCKS4, proxy)
	}

	tr := &http.Transport{Dial: dialSocksProxy}
	return tr;
}