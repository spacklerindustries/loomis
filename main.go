package main

import (
	"encoding/json"
	"flag"
	"fmt"
  "strings"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
  "net/http"
  "net"
  "path/filepath"
  "os/exec"
  "strconv"

  "github.com/pilebones/go-udev/netlink"
  "github.com/gorilla/mux"
  //"github.com/phayes/freeport"
  //"github.com/google/uuid"
)

var (
	filePath *string
)

var consoles = []byte(`[]`)
var stateFile = ""

type Exception struct {
  Message string `json:"message"`
}

type Record struct {
    UdevId   string `json:"udev"`
    DeviceId string `json:"device"`
}

var allRecords []Record

func init() {
	filePath = flag.String("file", "", "Optionnal input file path with matcher-rules (default: no matcher)")
}

func checkPort(portlow string, porthigh string) string {
  porta, _ := strconv.Atoi(porthigh)
  portb, _ := strconv.Atoi(portlow)
  port := ""
  fmt.Printf("-- %v %v\n",porta,portb)
  for i := porta; i <= portb; i++ {
    fmt.Printf("-- %v\n",i)
    port = strconv.Itoa(i)
    ln, err := net.Listen("tcp", ":" + port)
    if err != nil {
      fmt.Printf("Can't listen on port %q: %s\n", port, err)
      continue
    } else {
      _ = ln.Close()
      fmt.Printf("TCP Port %q is available\n", port)
      return port
    }
  }
  return ""
}

func main() {
	flag.Parse()
  stateFile = os.Getenv("STATE_FILE")
  if stateFile == "" {
    stateFile = "output.json"
    //log.Fatalln("STATE_FILE env var not set")
  }
  result, err := ioutil.ReadFile(stateFile) // just pass the file name
  if err != nil {
      fmt.Print(err)
  }
  consoles = result
  if err := json.Unmarshal([]byte(consoles), &allRecords); err != nil {
    log.Println(err)
  }
  /* gather what we know already from the records and check they exist on the system */
  dir := "/sys/devices" //all usb devices will be in here somewhere, lets find them
  for _, v := range allRecords {
    found := false
    err2 := filepath.Walk(dir, func(path string, info os.FileInfo, err2 error) error {
      if err2 != nil {
        fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", dir, err2)
        return err2
      }
      if strings.Contains(path, v.UdevId) {
        devid := strings.Split(v.DeviceId, "/")
        if strings.Contains(info.Name(), devid[1]) {
          log.Printf("%v %v", v.UdevId, v.DeviceId)
          found = true
          return nil
          /* our file matches the server */
        }
      }
      return nil
    })
    log.Printf("%v", found)
    /* if we don't find a match at all, then remove it from the records
      we can check for running services using this device and kill them here too
    */
    if found == false {
      allRecords = remove(allRecords, Record{UdevId: v.UdevId, DeviceId: v.DeviceId})
    }
    result, err := json.Marshal(allRecords)
    if err != nil {
      log.Println(err)
    }
    log.Printf("Result: %v", string(result))
    err = ioutil.WriteFile(stateFile, result, 0644)
    if err2 != nil {
		  fmt.Printf("error walking the path %q: %v\n", dir, err2)
	  }
  }

  /*
    poll greenskeeper for udev ids of plugged in caddies
    compare to what we know exists currently
    check if device matches etc, kill
  */
	matcher, err := getOptionnalMatcher()
	if err != nil {
		log.Fatalln(err.Error())
	}
	go monitor(matcher) //run udev nonblocking?

  /* api */
  r := mux.NewRouter()
  r.HandleFunc("/api/v1/consoles", ListConsoles).Methods("GET")
  log.Println("Ready to serve consoles!")
  log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", 9090), r))
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
      if (uevent.Env["SUBSYSTEM"] == "tty") {
        v := strings.Split(uevent.Env["DEVPATH"], "/")
        if (uevent.Env["ACTION"] == "add") {
          // v[8] ? may not always be v[8]
          allRecords = append(allRecords, Record{UdevId: v[8], DeviceId: "/dev/"+uevent.Env["DEVNAME"]})
          //s = append(s, string(input))
          log.Printf("%v %v", v[8], uevent.Env["DEVNAME"])
          port := checkPort("4300","4201")
          log.Println(port)
          cmd := exec.Command("/usr/bin/shellinaboxd","-t","-s","/:ben:ben:/:screen -D -R -S "+v[8]+" /dev/"+uevent.Env["DEVNAME"]+" 9600 -o","-p",port,"--background=/tmp/"+v[8]+".pid","--localhost-only")
          err2 := cmd.Start()
          cmd.Wait()
          if err2 != nil {
            log.Println(err2)
          }
        } else {
          //s = remove(s, string(input))
          allRecords = remove(allRecords, Record{UdevId: v[8], DeviceId: "/dev/"+uevent.Env["DEVNAME"]})
          log.Printf("%v %v", v[8], uevent.Env["DEVNAME"])
          pidnum, _ := exec.Command("cat", "/tmp/"+v[8]+".pid").CombinedOutput()
          cmd := exec.Command("kill",string(pidnum))
          err2 := cmd.Start()
          cmd.Wait()
          if err2 != nil {
            log.Println(err2)
          }
        }
        result, err := json.Marshal(allRecords)
        if err != nil {
          log.Println(err)
        }
        log.Printf("Result: %v", string(result))
        consoles = result
        err = ioutil.WriteFile(stateFile, result, 0644)
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

func ListConsoles(w http.ResponseWriter, req *http.Request) {
  w.Write(consoles)
}

func remove(s []Record, r Record) []Record {
    for i, v := range s {
        if v == r {
            return append(s[:i], s[i+1:]...)
        }
    }
    return s
}
