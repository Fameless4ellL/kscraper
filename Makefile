# Build the application
all: build test

build:
	@echo "Building..."


	@go build -o main.exe ./cmd/kuper

# Run the application
run:
	@go run ./cmd/kuper

# Test the application
test:
	@echo "Testing..."
	@go test ./... -v

# Clean the binary
clean:
	@echo "Cleaning..."
	@rm -f main
