all:
	mkdir -p bin
	GOOS=darwin GOARCH=amd64 go build -o ./bin/poptop-darwin-amd64
	GOOS=darwin GOARCH=arm64 go build -o ./bin/poptop-darwin-arm64

