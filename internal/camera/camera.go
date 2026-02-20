package camera

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

type Camera struct {
	Name    string
	RTSPURL string
	stopCh  chan struct{}
}

type StreamInfo struct {
	URL      string
	Protocol string
	Codecs   []string
}

func New(name, rtspURL string) *Camera {
	return &Camera{
		Name:    name,
		RTSPURL: rtspURL,
		stopCh:  make(chan struct{}),
	}
}

func (c *Camera) TestConnection(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", c.RTSPURL,
		"-frames:v", "1",
		"-f", "null",
		"-",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("connection timeout")
		}
		return fmt.Errorf("connection failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (c *Camera) Disconnect() {
	close(c.stopCh)
}

type DiscoveryResult struct {
	IP       string
	Port     int
	RTSPURLs []string
}

func DiscoverCameras(network string, timeout time.Duration) ([]DiscoveryResult, error) {
	var results []DiscoveryResult

	if network == "" {
		network = "192.168.1.0/24"
	}

	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return nil, fmt.Errorf("invalid network CIDR: %w", err)
	}

	commonPaths := []string{
		"/udp/av0_0",
		"/tcp/av0_0",
		"/live/ch0",
		"/live/ch00_0",
		"/stream1",
		"/h264",
		"/video1",
		"/cam/realmonitor?channel=1&subtype=0",
	}

	baseIP := ipnet.IP.Mask(ipnet.Mask)
	ones, _ := ipnet.Mask.Size()
	numHosts := 1 << (32 - ones)

	for i := 1; i < numHosts-1; i++ {
		ip := make(net.IP, 4)
		copy(ip, baseIP)
		for j := 0; j < 4; j++ {
			shift := uint((3 - j) * 8)
			ip[j] += byte((i >> shift) & 0xFF)
		}

		if !ipnet.Contains(ip) {
			continue
		}

		ipStr := ip.String()
		for _, port := range []int{554, 8554} {
			rtspURLs := probeRTSP(ipStr, port, commonPaths, timeout)
			if len(rtspURLs) > 0 {
				results = append(results, DiscoveryResult{
					IP:       ipStr,
					Port:     port,
					RTSPURLs: rtspURLs,
				})
			}
		}
	}

	return results, nil
}

func probeRTSP(ip string, port int, paths []string, timeout time.Duration) []string {
	var validURLs []string

	for _, path := range paths {
		rtspURL := fmt.Sprintf("rtsp://%s:%d%s", ip, port, path)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		cmd := exec.CommandContext(ctx, "ffprobe",
			"-rtsp_transport", "tcp",
			"-i", rtspURL,
			"-show_entries", "stream=codec_name",
			"-v", "quiet",
			"-of", "csv=p=0",
		)

		output, err := cmd.CombinedOutput()
		cancel()

		if err == nil && len(output) > 0 {
			validURLs = append(validURLs, rtspURL)
		}
	}

	return validURLs
}

func BuildRTSPURL(ip string, port int, username, password, path string) string {
	if port == 554 {
		return fmt.Sprintf("rtsp://%s:%s@%s%s", username, password, ip, path)
	}
	return fmt.Sprintf("rtsp://%s:%s@%s:%d%s", username, password, ip, port, path)
}

func ExtractCredentials(rtspURL string) (username, password, host, port, path string, err error) {
	parts := strings.Split(rtspURL, "://")
	if len(parts) != 2 {
		return "", "", "", "", "", fmt.Errorf("invalid RTSP URL format")
	}

	rest := parts[1]

	if atIdx := strings.Index(rest, "@"); atIdx != -1 {
		credentials := rest[:atIdx]
		rest = rest[atIdx+1:]

		if colonIdx := strings.Index(credentials, ":"); colonIdx != -1 {
			username = credentials[:colonIdx]
			password = credentials[colonIdx+1:]
		}
	}

	if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
		hostPort := rest[:slashIdx]
		path = "/" + rest[slashIdx+1:]

		if colonIdx := strings.Index(hostPort, ":"); colonIdx != -1 {
			host = hostPort[:colonIdx]
			port = hostPort[colonIdx+1:]
		} else {
			host = hostPort
			port = "554"
		}
	} else {
		host = rest
		port = "554"
		path = "/"
	}

	return
}

func DefaultRTSPPaths() []string {
	return []string{
		"/udp/av0_0",
		"/tcp/av0_0",
		"/live/ch0",
		"/stream1",
		"/h264",
		"/video1",
	}
}

func GetStreamInfo(rtspURL string, timeout time.Duration) (*StreamInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-show_entries", "stream=codec_name",
		"-show_entries", "format=format_name",
		"-v", "quiet",
		"-of", "csv=p=0",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to probe stream: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var codecs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "format") {
			codecs = append(codecs, line)
		}
	}

	return &StreamInfo{
		URL:      rtspURL,
		Protocol: "RTSP/TCP",
		Codecs:   codecs,
	}, nil
}
