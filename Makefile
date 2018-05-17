VERSION=0.1.9

all:
	@echo "make binary: build gobazel binary"
	@echo "make debian: build gobazel deb package"
	@echo "make fmt: format golang source code"

binary:
	go install -a -ldflags "-s -w -X main.version=${VERSION}" github.com/linuxerwang/gobazel

deb:
	go build -o debian/usr/bin/gobazel -a -ldflags "-s -w -X main.version=${VERSION}" github.com/linuxerwang/gobazel
	cd debian; fakeroot dpkg -b . ..

fmt:
	go fmt .
	go fmt ./conf
	go fmt ./exec
	go fmt ./gopathfs
