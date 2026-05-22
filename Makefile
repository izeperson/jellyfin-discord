GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=jellyfin-rpc
BINARY_UNIX=$(BINARY_NAME)
BINARY_WIN=$(BINARY_NAME).exe

# OS Detection
ifeq ($(OS),Windows_NT)
    BINARY_PATH=$(BINARY_WIN)
else
    BINARY_PATH=$(BINARY_UNIX)
endif

all: build

build:
	$(GOBUILD) -o $(BINARY_PATH) -v .

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v .

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_WIN) -v .

run: build
	./$(BINARY_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_UNIX) $(BINARY_WIN)

test:
	$(GOTEST) -v ./...
.PHONY: all build run clean test
