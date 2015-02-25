
KIBANA_VERSION=3.1.2

linux: kibana
	CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-s -w' -o traildash ./...

kibana:
	rm -rf kibana
	curl -s https://download.elasticsearch.org/kibana/kibana/kibana-$(KIBANA_VERSION).tar.gz | tar xvz -C .
	mv kibana-$(KIBANA_VERSION) kibana
	cp assets/config.js kibana/config.js
	cp assets/CloudTrail.json kibana/app/dashboards/default.json
	GOOS=linux go-bindata -pkg="main" kibana/...
	rm -rf kibana

docker: linux
	docker build -t appliedtrust/traildash .

all: kibana linux docker 
