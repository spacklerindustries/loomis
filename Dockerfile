FROM phusion/baseimage
# use base image to my_init
# https://github.com/neomindryan/rpi-baseimage-docker < look to make this more up to date
#FROM ubuntu

RUN apt-get update && apt-get install nginx shellinabox screen -y
RUN adduser nobody dialout

EXPOSE 8080

RUN mkdir -p /app/loomis/bin
RUN mkdir -p /app/loomis/run
RUN mkdir -p /app/loomis/config

COPY main /app/loomis/bin/.
COPY nginx.conf /etc/nginx/nginx.conf
COPY nginx-template.conf.tpl /app/loomis/nginx-template.conf.tpl
COPY entrypoint.sh /app/loomis/bin/.
RUN chmod +x /app/loomis/bin/entrypoint.sh

VOLUME /app/loomis/config/

ENTRYPOINT ["/sbin/my_init"]

CMD [ "/app/loomis/bin/entrypoint.sh" ]

## not ideal way to do it
#docker run --rm --privileged -v /dev:/dev -v $(pwd)/vol:/app/loomis/config/:rw -p 8080:8080 --device-cgroup-rule='c 189:* rmw' --name loomis loomis
