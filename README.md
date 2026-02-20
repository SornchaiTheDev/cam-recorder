# IP Camera Recorder

A Go-based video recorder for IP cameras with RTSP support. Supports multiple cameras with a grid view web interface.

## Features

- **Multi-camera support** - Up to 16 cameras simultaneously
- **Grid view dashboard** - View all cameras at once
- **Continuous recording** - Segmented MP4 files per camera
- **MJPEG live streaming** - Low-latency browser viewing
- **Per-camera storage** - Organized recordings by camera
- **Automatic file rotation** - Time-based retention policy
- **Auto-reconnection** - Handles stream failures gracefully
- **REST API** - Control cameras programmatically

## Requirements

- Go 1.21+
- FFmpeg (for video processing)

## Quick Start

```bash
# Build
make build

# Edit config.yaml with your camera details
vim config.yaml

# Run
./bin/cam-recorder -config config.yaml
```

## Configuration

Edit `config.yaml`:

```yaml
cameras:
  - name: "Front Door"
    rtsp_url: "rtsp://admin:password@192.168.1.100:554/udp/av0_0"
    enabled: true
  - name: "Backyard"
    rtsp_url: "rtsp://admin:password@192.168.1.101:554/udp/av0_0"
    enabled: true
  - name: "Garage"
    rtsp_url: "rtsp://admin:password@192.168.1.102:554/udp/av0_0"
    enabled: false

recording:
  segment_duration: 5m        # Duration of each segment
  retention_days: 7           # Delete files older than this
  output_dir: "./recordings"  # Where to store recordings
  format: "mp4"               # Output format

server:
  host: "0.0.0.0"
  port: 8080
```

## Storage Structure

```
recordings/
├── Front_Door/
│   ├── Front_Door_20260220_100000.mp4
│   └── Front_Door_20260220_100500.mp4
├── Backyard/
│   └── Backyard_20260220_100000.mp4
└── Garage/
    └── ...
```

## RTSP URL Formats

### Vstarcam
```
rtsp://user:pass@IP:554/udp/av0_0
rtsp://user:pass@IP:554/tcp/av0_0
rtsp://user:pass@IP:554/live/ch0
```

### Other Common Formats
```
rtsp://user:pass@IP:554/stream1
rtsp://user:pass@IP:554/h264
rtsp://user:pass@IP:554/cam/realmonitor?channel=1&subtype=0
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Grid view dashboard |
| `GET /camera/:name` | Single camera detail |
| `GET /live/:name` | MJPEG stream for camera |
| `GET /recordings` | List all recordings (JSON) |
| `GET /recordings/list` | Recordings page |
| `GET /recordings/list?camera=Front Door` | Filter by camera |
| `GET /recordings/download/:camera/:filename` | Download recording |
| `GET /recordings/play/:camera/:filename` | Play recording |
| `DELETE /recordings/:camera/:filename` | Delete recording |
| `GET /api/status` | Status of all cameras |
| `GET /api/status/:name` | Single camera status |
| `GET /api/storage` | Storage statistics |
| `POST /api/camera/:name/start` | Start recording |
| `POST /api/camera/:name/stop` | Stop recording |

## Simulating a Camera with Webcam

Use MediaMTX + FFmpeg to simulate an RTSP camera:

```bash
# Install MediaMTX
wget https://github.com/bluenviron/mediamtx/releases/download/v1.9.0/mediamtx_v1.9.0_linux_amd64.tar.gz
tar -xzf mediamtx_v1.9.0_linux_amd64.tar.gz

# Start RTSP server
./mediamtx &

# Push webcam to RTSP
ffmpeg -f v4l2 -i /dev/video0 -c:v libx264 -preset ultrafast -tune zerolatency -f rtsp rtsp://localhost:8554/webcam
```

Then add to config.yaml:
```yaml
cameras:
  - name: "Webcam Test"
    rtsp_url: "rtsp://localhost:8554/webcam"
    enabled: true
```

## Build

```bash
make build
./bin/cam-recorder -config config.yaml
```

## Development

```bash
make deps     # Install dependencies
make run      # Run with config.yaml
make fmt      # Format code
make lint     # Run linter
```

## License

MIT
