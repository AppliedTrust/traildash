
linux: kibana
	CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-s -w' -o traildash ./...

kibana:
	rm -rf kibana
	curl -s https://download.elasticsearch.org/kibana/kibana/kibana-3.1.2.tar.gz | tar xvz -C .
	mv kibana-3.1.2 kibana
	cp assets/config.js kibana/config.js
	cp assets/CloudTrail.json kibana/app/dashboards/default.json
	GOOS=linux go-bindata -pkg="main" kibana/...
	rm -rf kibana

docker: linux
	docker build -t appliedtrust/traildash .

all: kibana linux docker 
