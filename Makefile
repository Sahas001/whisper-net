build:
	go build -o bin/app .

bootstrap-peer:
	./bin/app bootstrap

client-peer:
	./bin/app client ./bootstrap_peer.addr

clean:
	rm -rf bin/

.PHONY: build run clean bootstrap-peer client-peer
