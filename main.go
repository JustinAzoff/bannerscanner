package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	flag "github.com/spf13/pflag"
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

type ScanConfiguration struct {
	include []string
	exclude []string
	ports   []int
	ScanParams
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
	log.Debug().Str("hostport", hostport).Msg("scanning")
	conn, err := net.DialTimeout("tcp", hostport, sr.dialTimeout)
	if err != nil {
		res.err = err
		return res
	}
	defer conn.Close()
	bannerBuffer := make([]byte, 4096)
	conn.SetDeadline(time.Now().Add(sr.bannerTimeout))
	conn.Write([]byte("\r\n\r\n"))
	n, err := conn.Read(bannerBuffer)
	if err == nil {
		banner = string(bannerBuffer[:n])
	}
	res.open = true
	res.banner = banner
	return res
}

func makeScans(sc ScanConfiguration) chan MultiPortScanRequest {
	ch := make(chan MultiPortScanRequest, 1000)
	go func() {
		hosts, err := EnumerateHosts(sc.include, sc.exclude)
		if err != nil {
			log.Fatal().Err(err).Msg("Invalid scan configuration")
		}
		for _, host := range hosts {
			msr := MultiPortScanRequest{
				host:       host,
				ports:      sc.ports,
				ScanParams: sc.ScanParams,
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
			if d, _ := isDone(ctx); d {
				goto done
			}
			out <- msr
		}
	done:
		close(out)
	}()
	return out
}

func isDone(ctx context.Context) (bool, struct{}) {
	select {
	case r := <-ctx.Done():
		return true, r
	default:
		return false, struct{}{}
	}

}

func scanWorker(ctx context.Context, ch chan MultiPortScanRequest) {
	for s := range ch {
		for _, sr := range s.Expand() {
			done, _ := isDone(ctx)
			if done {
				return
			}
			res := ScanPort(sr)
			if res.open {
				log.Info().
					Str("state", "open").
					Str("host", res.host).
					Int("port", res.port).
					Str("banner", res.banner).
					Msg("found service")
			} else if res.err != nil {
				log.Debug().
					Str("state", "error").
					Str("host", res.host).
					Int("port", res.port).
					Err(res.err).
					Msg("error scanning")
			}
		}
	}
}

func startScanners(ctx context.Context, ch chan MultiPortScanRequest, n int) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			scanWorker(ctx, ch)
			wg.Done()
		}()
	}
	wg.Wait()
}

func main() {

	scanRate := flag.Int("rate", 1000, "rate in attempts/sec")
	ports := flag.IntSliceP("port", "p", []int{}, "ports to scan")
	dialTimeout := flag.Duration("timeout", 2*time.Second, "Scan connection timeout")
	bannerTimeout := flag.Duration("banner-timeout", 2*time.Second, "timeout when fetching banner")
	debug := flag.Bool("debug", false, "sets log level to debug")
	pretty := flag.Bool("pretty", false, "use pretty logs")
	exclude := flag.StringSliceP("exclude", "x", []string{}, "cidr blocks to exclude")

	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Logger = log.Output(os.Stdout)
	if *pretty {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}

	sc := ScanConfiguration{
		ports:   *ports,
		include: flag.Args(),
		exclude: *exclude,
		ScanParams: ScanParams{
			dialTimeout:   *dialTimeout,
			bannerTimeout: *bannerTimeout,
		},
	}
	log.Debug().Msgf("Scanning: %+v", sc)

	if len(*ports) == 0 {
		log.Fatal().Msg("No ports specified")
		return
	}

	scans := makeScans(sc)
	rl := rate.NewLimiter(rate.Limit(*scanRate), *scanRate)
	ctx, _ := context.WithCancel(context.Background())
	//go func() {
	//	time.Sleep(10 * time.Second)
	//	cancel()
	//}()
	limited := rateLimitScans(ctx, scans, rl)
	startScanners(ctx, limited, *scanRate+50)
}
