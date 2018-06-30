# Console server

This is the server that runs where the console server will run

Requires the following
* nginx
* shellinabox
* screen

```
docker build -t spacklerind/loomis .
docker run -it --rm --privileged -v /dev:/dev -v $(pwd)/vol:/app/loomis/config/:rw -p 8080:8080 -p 8081:8081 --device-cgroup-rule='c 189:* rmw' --name loomis spacklerind/loomis
```


# devices
If inserted, slot order will be similar to this
```
1-1.3.<slotnumber>:1.0
1-1.3.<slotnumber>:1.0
1-1.3.<slotnumber>:1.0
1-1.3.4.<slotnumber>:1.0
1-1.3.4.<slotnumber>:1.0
1-1.3.4.<slotnumber>:1.0
1-1.3.4.4.<slotnumber>:1.0
1-1.3.4.4.<slotnumber>:1.0
1-1.3.4.4.<slotnumber>:1.0
1-1.3.4.4.4.<slotnumber>:1.0
1-1.3.4.4.4.<slotnumber>:1.0
1-1.3.4.4.4.<slotnumber>:1.0
```


## USB Ports
```
1-1.2.4.3:1.0
1-1.3.4.3:1.0
1-1.4.4.3:1.0
1-1.5.4.3:1.0
1-1.2.4.4.3:1.0
1-1.3.4.4.3:1.0
1-1.4.4.4.3:1.0
1-1.5.4.4.3:1.0
```
