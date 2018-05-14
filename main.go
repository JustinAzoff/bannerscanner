package main

import (
	"fmt"
	"log"
	"net"
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
	//log.Printf("Scanning %s:%d", host, port)
	res := ScanResult{host: sr.host, port: sr.port}
	var banner string
	hostport := sr.HostPort()
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
func main() {
	rl := rate.NewLimiter(20, 5)
	for i := 0; i < 90; i++ {
		r := rl.Reserve()
		if !r.OK() {
			// Not allowed to act! Did you remember to set lim.burst to be > 0 ?
			return
		}
		time.Sleep(r.Delay())
		res := ScanPort(ScanRequest{
			host:       "192.168.2.1",
			port:       i,
			ScanParams: DefaultScanParams,
		})
		if res.open {
			log.Printf("%s %d %s", res.host, res.port, res.banner)
		}
	}
}
