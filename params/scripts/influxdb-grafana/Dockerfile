FROM centos 
MAINTAINER Nerthus <nerthus@nerthus.io>

RUN curl -o grafana.rpm https://s3-us-west-2.amazonaws.com/grafana-releases/release/grafana-5.0.3-1.x86_64.rpm
RUN yum install -y grafana.rpm
RUN curl -o influxdb.rpm https://dl.influxdata.com/influxdb/releases/influxdb-1.5.1.x86_64.rpm
RUN yum install -y influxdb.rpm
RUN rm -fr grafana.rpm influxdb.rpm
RUN yum clean all

WORKDIR /root/

ADD start_service.sh /root/start_service.sh

ENTRYPOINT sh start_service.sh
