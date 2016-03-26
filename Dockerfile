FROM golang

ADD dist/linux/amd64/traildash /usr/local/traildash/traildash

ADD assets/start /root/start

RUN chmod 755 /root/start /usr/local/traildash/traildash

CMD ["/root/start"]


