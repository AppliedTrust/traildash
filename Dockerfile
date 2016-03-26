FROM golang:1.6

ADD dist/linux/amd64/traildash /usr/local/traildash/traildash

RUN chmod 755 /usr/local/traildash/traildash

CMD ["/usr/local/traildash/traildash"]


