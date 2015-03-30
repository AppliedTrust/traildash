TAG:=`git describe --abbrev=0 --tags`
KIBANA_VERSION=3.1.2

deps:
	# points links to new locations
	go get github.com/awslabs/aws-sdk-go/s3
	go get github.com/awslabs/aws-sdk-go/sqs
	glock sync github.com/appliedtrust/traildash

dist-clean:
	rm -rf dist
	rm -rf kibana

release: deps dist
	tar -cvzf traildash-linux-amd64-$(TAG).tar.gz -C dist/linux/amd64 traildash
	tar -cvzf traildash-darwin-amd64-$(TAG).tar.gz -C dist/darwin/amd64 traildash

dist: dist-clean kibana 
	mkdir -p dist/linux/amd64 && GOOS=linux GOARCH=amd64 go build -tags netgo -o dist/linux/amd64/traildash
	mkdir -p dist/darwin/amd64 && GOOS=darwin GOARCH=amd64 go build -tags netgo -o dist/darwin/amd64/traildash

kibana:
	curl -s https://download.elasticsearch.org/kibana/kibana/kibana-$(KIBANA_VERSION).tar.gz | tar xvz -C .
	mv kibana-$(KIBANA_VERSION) kibana
	cp assets/config.js kibana/config.js
	cp assets/CloudTrail.json kibana/app/dashboards/default.json
	GOOS=linux go-bindata -pkg="main" kibana/...
	rm -rf kibana

docker: dist
	docker build -t appliedtrust/traildash .


