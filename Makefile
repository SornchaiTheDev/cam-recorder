.PHONY: build run clean test deps docker-build docker-run install-ffmpeg fmt lint build-all build-linux build-windows build-darwin

APP_NAME=cam-recorder
VERSION?=dev
BUILD_DIR=./bin
MAIN_PATH=./cmd/main.go

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)

run:
	go run $(MAIN_PATH) -config config.yaml

dev:
	go run $(MAIN_PATH) -config config.yaml &

clean:
	rm -rf $(BUILD_DIR)
	rm -rf recordings/*.mp4

test:
	go test -v ./...

deps:
	go mod tidy
	go mod download

docker-build:
	docker build -t $(APP_NAME):$(VERSION) .

docker-run:
	docker run -p 8080:8080 -v $(PWD)/recordings:/recordings $(APP_NAME):$(VERSION)

install-ffmpeg:
	sudo apt-get update && sudo apt-get install -y ffmpeg

fmt:
	go fmt ./...

lint:
	go vet ./...

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 $(MAIN_PATH)

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
	GOOS=windows GOARCH=arm64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-windows-arm64.exe $(MAIN_PATH)

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)

build-all: deps
	mkdir -p $(BUILD_DIR)
	$(MAKE) build-linux
	$(MAKE) build-windows
	$(MAKE) build-darwin

.DEFAULT_GOAL := build
