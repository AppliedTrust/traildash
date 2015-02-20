FROM ubuntu:14.04
MAINTAINER AppliedTrust

RUN apt-get update && apt-get -y install openjdk-7-jre-headless wget && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
RUN wget -q -O /usr/src/elasticsearch.deb https://download.elasticsearch.org/elasticsearch/elasticsearch/elasticsearch-1.4.2.deb && dpkg -i /usr/src/elasticsearch.deb

#
RUN echo "# CORS settings:\nhttp.cors.enabled: true\nhttp.cors.allow-origin: true\n" >> /etc/elasticsearch/elasticsearch.yml
ADD docker_assets/traildash /usr/local/traildash/traildash 
ADD docker_assets/kibana-3.1.2 /usr/local/traildash/kibana 
ADD docker_assets/config.js /usr/local/traildash/kibana/config.js
ADD docker_assets/CloudTrail.json /usr/local/traildash/kibana/app/dashboards/default.json

RUN /usr/local/traildash/traildash --version > /usr/local/traildash/kibana/traildash-version

#
ADD docker_assets/start /root/start
RUN chmod 755 /root/start /usr/local/traildash/traildash

EXPOSE 7000
CMD ["/root/start"]


