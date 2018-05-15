package main

import (
	"context"
	"fmt"
	"math/rand"
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
	include   []string
	exclude   []string
	ports     []int
	parallel  bool
	randomize bool
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
		if sc.randomize {
			rand.Shuffle(len(hosts), func(i, j int) {
				hosts[i], hosts[j] = hosts[j], hosts[i]
			})
		}
		for _, host := range hosts {
			if !sc.parallel {
				msr := MultiPortScanRequest{
					host:       host,
					ports:      sc.ports,
					ScanParams: sc.ScanParams,
				}
				ch <- msr
			} else {
				for _, p := range sc.ports {
					msr := MultiPortScanRequest{
						host:       host,
						ports:      []int{p},
						ScanParams: sc.ScanParams,
					}
					ch <- msr
				}
			}
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

func expandPorts(portSpecs []string) ([]int, error) {
	var ports []int
	for _, spec := range portSpecs {
		some, err := EnumeratePorts(spec)
		if err != nil {
			return ports, err
		}
		ports = append(ports, some...)
	}
	return ports, nil
}

func main() {

	scanRate := flag.Int("rate", 1000, "rate in attempts/sec")
	portspecs := flag.StringSliceP("port", "p", []string{}, "ports to scan. ex: 80,443,8000-8100")
	dialTimeout := flag.Duration("timeout", 2*time.Second, "Scan connection timeout")
	bannerTimeout := flag.Duration("banner-timeout", 2*time.Second, "timeout when fetching banner")
	debug := flag.Bool("debug", false, "sets log level to debug")
	pretty := flag.Bool("pretty", false, "use pretty logs")
	exclude := flag.StringSliceP("exclude", "x", []string{}, "cidr blocks to exclude")
	parallel := flag.Bool("parallel", false, "Scan multiple ports on each host in parallel")

	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Logger = log.Output(os.Stdout)
	if *pretty {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}
	ports, err := expandPorts(*portspecs)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid port specification")
	}

	sc := ScanConfiguration{
		ports:     ports,
		include:   flag.Args(),
		exclude:   *exclude,
		parallel:  *parallel,
		randomize: true,
		ScanParams: ScanParams{
			dialTimeout:   *dialTimeout,
			bannerTimeout: *bannerTimeout,
		},
	}
	log.Debug().Msgf("Scanning: %+v", sc)

	if len(ports) == 0 {
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
