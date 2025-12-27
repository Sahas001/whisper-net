build:
	go build -o bin/app .

run:
	./bin/app

clean:
	rm -rf bin/

.PHONY: build run clean
