build:
	go build -o bin/aether cmd/aether/main.go

run: build
	./bin/aether -config aether.yaml