#TO_BUILD: docker build -t docker.internal.community.nw.ops.here.com/traildash:latest .

FROM    ubuntu:14.04
MAINTAINER Holger Morch <holger.morch@here.com>

RUN     apt-get update && apt-get -y install openjdk-7-jre-headless wget python python-pip && pip install boto3 && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
RUN     wget -q -O /usr/src/elasticsearch.deb https://download.elasticsearch.org/elasticsearch/elasticsearch/elasticsearch-1.4.2.deb && dpkg -i /usr/src/elasticsearch.deb

#
RUN     echo "# CORS settings:\nhttp.cors.enabled: true\nhttp.cors.allow-origin: true\n" >> /etc/elasticsearch/elasticsearch.yml
ADD     traildash /usr/local/traildash/traildash

# 
ADD     backfill.py /usr/local/bin/

#
ADD     start /root/start
RUN     chmod 755 /root/start /usr/local/traildash/traildash /usr/local/bin/backfill.py

EXPOSE 7000
CMD ["/root/start"]

ADD     Dockerfile /
