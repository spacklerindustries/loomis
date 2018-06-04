FROM ubuntu
# ARM
#FROM arm32v7/ubuntu

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
COPY htpass.tpl /app/loomis/htpass.tpl
COPY tini /app/tini
COPY entrypoint.sh /app/loomis/bin/.
RUN chmod +x /app/loomis/bin/entrypoint.sh

VOLUME /app/loomis/config/

ENV GK_TOKEN="d42a152bff711f187479d8613ccb47925d82b21a"
ENV GK_SERVER="http://10.1.1.1:8080"
ENV HTTP_PORT="8080"
ENV CONSOLES_PORT="8081"
#these are the ports to specify for exposing using -p which ports you use
# eg if -p 9090:8080, then DOCKER_HTTP_PORT needs to be 9090
# eg if -p 9091:8081, then DOCKER_CONSOLES_PORT needs to be 9091
ENV DOCKER_HTTP_PORT="9090"
ENV DOCKER_CONSOLES_PORT="9091"
ENV LOOMIS_SERVER="http://10.1.1.1"

## TINI
ENV TINI_VERSION v0.18.0
#ARM
#ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-armhf /tini
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini
RUN chmod +x /tini
ENTRYPOINT ["/tini", "--"]

#CMD [" /app/loomis/bin/main" ]
CMD [ "/app/loomis/bin/entrypoint.sh" ]

## not ideal way to do it
#docker build -t spacklerind/loomis .
#docker run -it --rm --privileged -v /dev:/dev -v $(pwd)/vol:/app/loomis/config/:rw -p 8080:8080 -p 8081:8081 --device-cgroup-rule='c 189:* rmw' --name loomis spacklerind/loomis
