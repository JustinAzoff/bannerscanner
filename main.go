package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type ScanParams struct {
	dialTimeout   time.Duration
	bannerTimeout time.Duration
}

var DefaultScanParams = ScanParams{
	dialTimeout:   2 * time.Second,
	bannerTimeout: 2 * time.Second,
}

type ScanRequest struct {
	host string
	port int
	ScanParams
}

type MultiPortScanRequest struct {
	host  string
	ports []int
	ScanParams
}

func (m *MultiPortScanRequest) Expand() []ScanRequest {
	var scans []ScanRequest
	for _, p := range m.ports {
		scans = append(scans, ScanRequest{
			host:       m.host,
			port:       p,
			ScanParams: m.ScanParams,
		})
	}
	return scans
}

func (sr *ScanRequest) HostPort() string {
	return fmt.Sprintf("%s:%d", sr.host, sr.port)
}

type ScanResult struct {
	host   string
	port   int
	open   bool
	err    error
	banner string
}

func (s *ScanResult) String() string {
	return fmt.Sprintf("%s:%d ok=%v err=%s banner=%q",
		s.host, s.port, s.open, s.err, s.banner)
}

func ScanPort(sr ScanRequest) ScanResult {
	res := ScanResult{host: sr.host, port: sr.port}
	var banner string
	hostport := sr.HostPort()
	log.Printf("Scanning %s", hostport)
	conn, err := net.DialTimeout("tcp", hostport, sr.dialTimeout)
	if err != nil {
		//res.err = err
		return res
	}
	defer conn.Close()
	bannerBuffer := make([]byte, 4096)
	conn.SetDeadline(time.Now().Add(sr.bannerTimeout))
	n, err := conn.Read(bannerBuffer)
	if err == nil {
		banner = string(bannerBuffer[:n])
	}
	res.open = true
	res.banner = banner
	return res
}

func makeScans() chan MultiPortScanRequest {
	ch := make(chan MultiPortScanRequest, 1000)
	go func() {
		for i := 30; i < 255; i++ {
			host := fmt.Sprintf("192.168.2.%d", i)
			msr := MultiPortScanRequest{
				host:       host,
				ports:      []int{22, 80, 443},
				ScanParams: DefaultScanParams,
			}
			ch <- msr
		}
		close(ch)
	}()
	return ch
}

func rateLimitScans(ctx context.Context, ch chan MultiPortScanRequest, rl *rate.Limiter) chan MultiPortScanRequest {
	out := make(chan MultiPortScanRequest, 2000)
	go func() {
		for msr := range ch {
			rl.Wait(ctx)
			out <- msr
		}
		close(out)
	}()
	return out
}

func scanStuff(ch chan MultiPortScanRequest) {
	for s := range ch {
		for _, sr := range s.Expand() {
			res := ScanPort(sr)
			if res.open {
				log.Printf("%s %d %s", res.host, res.port, res.banner)
			}
		}
	}
}

func startScanners(ch chan MultiPortScanRequest, n int) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			scanStuff(ch)
			wg.Done()
		}()
	}
	wg.Wait()
}

func main() {
	scans := makeScans()
	rl := rate.NewLimiter(20, 5)
	limited := rateLimitScans(context.TODO(), scans, rl)
	startScanners(limited, 20)
}
