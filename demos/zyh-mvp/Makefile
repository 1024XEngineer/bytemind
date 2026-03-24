build:
	go build -o bin/forgecli.exe ./cmd/forgecli

run:
	go run ./cmd/forgecli version

test:
	go test ./...

lint:
	go vet ./...

clean:
	powershell -Command "Remove-Item -Recurse -Force bin -ErrorAction SilentlyContinue"
