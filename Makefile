VERSION = v$(shell cat .version)

test:
	go test -v -race ./...

cover:
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

release:
	git push
	git push origin $(VERSION)