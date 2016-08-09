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
	"github.com/cheggaaa/pb"
	"io"
	"bytes"
	"fmt"
)

const TIMEOUT = time.Duration(5 * time.Second)
const WORKER_THREADS = 30
//downloadable at: https://dev.maxmind.com/geoip/geoip2/geolite2/
const GEO_IP_FILE = "GeoLite2-Country.mmdb"
//ip of google
const TEST_TARGET = "http://216.58.210.14"

var REDIRECT_ERROR = errors.New("Host redirected to different target")

type logWriter struct {
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(string(bytes))
}

func main() {
	log.SetFlags(0)
	log.SetOutput(new(logWriter))

	log.Println("Loading input")

	toTest := make(chan Proxy)
	working := make(chan Proxy, 256)
	done := make(chan bool)

	var testIndex uint32 = 0

	var wg sync.WaitGroup

	for i := 0; i < WORKER_THREADS; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				proxy, more := <-toTest
				if !more {
					break
				}

				index := atomic.AddUint32(&testIndex, 1)

				log.Println(index, "Testing", proxy.host)
				if proxy.isOnline() {
					log.Println(index, "Working Socks", proxy.socks5, proxy.host, proxy.time, "ms")
					working <- proxy
				}
			}
		}()
	}

	go writeWorkingProxies(working, done)

	input, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	defer input.Close()

	totalLines, err := lineCounter(input)
	if err != nil {
		log.Fatal(err)
	}

	input.Seek(0, 0)
	bar := pb.StartNew(totalLines)

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

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()

		ip := net.ParseIP(strings.Split(line, ":")[0])
		countryIso := ""
		if db != nil {
			country, err := db.Country(ip)

			if err == nil {
				countryIso = country.Country.IsoCode
			}
		}

		toTest <- Proxy{host: line, country:countryIso}
		bar.Increment()
	}

	if err = scanner.Err(); err != nil {
		log.Fatal(err)
	}

	close(toTest)
	wg.Wait()
	//everything is written to the channel
	close(working)

	<-done
	bar.Finish()
}

type Proxy struct {
	host    string
	country string
	socks5  bool
	time    int64
}

func (proxy *Proxy) isOnline() bool {
	if works, time := testSocksProxy(proxy.host, true); works {
		proxy.socks5 = true
		proxy.time = time
		return true
	}

	if works, time := testSocksProxy(proxy.host, false); works {
		proxy.time = time
		return true
	}

	return false
}

func writeWorkingProxies(working <-chan Proxy, done chan <- bool) {
	if _, err := os.Stat(os.Args[2]); err == nil {
		// path doesn't exist does not exist
		os.Remove(os.Args[2])
	}

	output, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}

	defer output.Close()

	writer := bufio.NewWriter(output)
	for {
		proxy, more := <-working
		if !more {
			break
		}

		_, err := writer.WriteString(proxy.host + "\n")
		if err != nil {
			log.Fatal(err)
		}
	}

	writer.Flush()
	done <- true
}

func testSocksProxy(line string, socks5 bool) (bool, int64) {
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
	responseTime := end.Sub(start).Nanoseconds() / time.Millisecond.Nanoseconds()
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

func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 32 * 1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}