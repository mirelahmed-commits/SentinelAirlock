APP=airlock
PKG=./cmd/airlock
VERSION?=dev
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE?=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PREFIX?=/usr/local/bin
LDFLAGS=-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build install release-artifacts

build:
	go build -ldflags "$(LDFLAGS)" -o $(APP) $(PKG)

install:
	go build -ldflags "$(LDFLAGS)" -o $(APP) $(PKG)
	mkdir -p $(PREFIX)
	cp $(APP) $(PREFIX)/$(APP)
	@echo "installed to $(PREFIX)/$(APP)"

release-artifacts:
	mkdir -p dist
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/airlock-$(VERSION)-darwin-amd64 $(PKG)
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/airlock-$(VERSION)-darwin-arm64 $(PKG)
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/airlock-$(VERSION)-linux-amd64 $(PKG)
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/airlock-$(VERSION)-linux-arm64 $(PKG)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/airlock-$(VERSION)-windows-amd64.exe $(PKG)
	@printf '{\n  "version": "%s",\n  "commit": "%s",\n  "build_date": "%s"\n}\n' "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)" > dist/build_info.json
