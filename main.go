package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pilebones/go-udev/netlink"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	filePath *string
)

func init() {
	filePath = flag.String("file", "", "Optionnal input file path with matcher-rules (default: no matcher)")
}

func main() {
	flag.Parse()

	matcher, err := getOptionnalMatcher()
	if err != nil {
		log.Fatalln(err.Error())
	}
	monitor(matcher)
}

func monitor(matcher netlink.Matcher) {
	log.Println("Monitoring UEvent kernel message to user-space...")

	conn := new(netlink.UEventConn)
	if err := conn.Connect(); err != nil {
		log.Fatalln("Unable to connect to Netlink Kobject UEvent socket")
	}
	defer conn.Close()

	queue := make(chan netlink.UEvent)
	errors := make(chan error)
	quit := conn.Monitor(queue, errors, matcher)
	// Signal handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("Exiting monitor mode...")
		quit <- struct{}{}
		os.Exit(0)
	}()
	// Handling message from queue
	for {
		select {
		case uevent := <-queue:
      for key, value := range uevent.Env {
          //fmt.Fprintf(b, "%s=\"%s\"\n", key, value)
          if (key == "DEVNAME" && uevent.Env["SUBSYSTEM"] == "tty") {
            //we got a usb tty device
            log.Printf(value)
          }
      }
		case err := <-errors:
			log.Printf("ERROR: %v", err)
		}
	}

}

func getOptionnalMatcher() (matcher netlink.Matcher, err error) {
	if filePath == nil || *filePath == "" {
		return nil, nil
	}

	stream, err := ioutil.ReadFile(*filePath)
	if err != nil {
		return nil, err
	}

	if stream == nil {
		return nil, fmt.Errorf("Empty, no rules provided in \"%s\", err: %s", *filePath, err.Error())
	}

	var rules netlink.RuleDefinitions
	if err := json.Unmarshal(stream, &rules); err != nil {
		return nil, fmt.Errorf("Wrong rule syntax in \"%s\", err: %s", *filePath, err.Error())
	}

	return &rules, nil
}
