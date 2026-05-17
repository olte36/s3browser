BINARY := s3browser
BUILD_DIR := out

.PHONY: build clean test

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) .

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)
