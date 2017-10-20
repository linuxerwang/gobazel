VERSION=0.1.7

all:
	@echo "make binary: build gobazel binary"

binary:
	go install -a -ldflags "-s -w -X main.version=${VERSION}" github.com/linuxerwang/gobazel

fmt:
	go fmt .
	go fmt ./conf
	go fmt ./exec
	go fmt ./gopathfs
