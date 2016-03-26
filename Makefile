TAG:=`git describe --abbrev=0 --tags`

deps:
	go get github.com/aws/aws-sdk-go
	glock sync github.com/appliedtrust/traildash

dist-clean:
	rm -rf dist

release: deps dist
	tar -cvzf traildash-linux-amd64-$(TAG).tar.gz -C dist/linux/amd64 traildash
	tar -cvzf traildash-darwin-amd64-$(TAG).tar.gz -C dist/darwin/amd64 traildash

dist: dist-clean
	mkdir -p dist/linux/amd64 && GOOS=linux GOARCH=amd64 go build -tags netgo -o dist/linux/amd64/traildash
	mkdir -p dist/darwin/amd64 && GOOS=darwin GOARCH=amd64 go build -tags netgo -o dist/darwin/amd64/traildash

docker: dist
	docker build -t cloud-analytics/traildash .


