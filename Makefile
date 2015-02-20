linux:
	CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-s -w' -o docker_assets/traildash traildash.go

kibana:
	rm -rf docker_assets/kibana-3.1.2
	curl -s https://download.elasticsearch.org/kibana/kibana/kibana-3.1.2.tar.gz | tar xvz -C docker_assets

docker: linux kibana
	docker build -t appliedtrust/traildash .

