FROM phusion/baseimage
# use base image to my_init
# https://github.com/neomindryan/rpi-baseimage-docker < look to make this more up to date
#FROM ubuntu

RUN apt-get update && apt-get install nginx shellinabox screen -y
RUN adduser nobody dialout

EXPOSE 8080
EXPOSE 8081

RUN mkdir -p /app/loomis/bin
RUN mkdir -p /app/loomis/run
RUN mkdir -p /app/loomis/config

COPY main /app/loomis/bin/.
COPY nginx.conf /etc/nginx/nginx.conf
COPY nginx-template.conf.tpl /app/loomis/nginx-template.conf.tpl
COPY entrypoint.sh /app/loomis/bin/.
RUN chmod +x /app/loomis/bin/entrypoint.sh

VOLUME /app/loomis/config/

ENV GK_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjowLCJ1c2VyaWQiOjEsInVzZXJuYW1lIjoiYWRtaW4ifQ.GQZFA7KICyo3-5xW4FOuwoNyJtjuGCQpIzzcPNgV-vM"
ENV GK_SERVER="http://10.1.1.1:8080"
ENV HTTP_PORT="8080"
ENV CONSOLES_PORT="8081"
#these are the ports to specify for exposing using -p which ports you use
# eg if -p 9090:8080, then DOCKER_HTTP_PORT needs to be 9090
# eg if -p 9091:8081, then DOCKER_CONSOLES_PORT needs to be 9091
ENV DOCKER_HTTP_PORT="9090"
ENV DOCKER_CONSOLES_PORT="9091"
ENV LOOMIS_SERVER="http://10.1.1.1"

ENTRYPOINT ["/sbin/my_init"]

CMD [ "/app/loomis/bin/entrypoint.sh" ]

## not ideal way to do it
#docker run --rm --privileged -v /dev:/dev -v $(pwd)/vol:/app/loomis/config/:rw -p 8080:8080 -p 8081:8081 --device-cgroup-rule='c 189:* rmw' --name loomis loomis
