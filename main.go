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
  "html/template"
  "errors"

  "github.com/pilebones/go-udev/netlink"
  "github.com/gorilla/mux"
  "github.com/google/uuid"
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

type templatevars struct {
	NginxPort string
	ShellPort string
	NginxUuid string
}

var allRecords []Record

func init() {
	filePath = flag.String("file", "", "Optionnal input file path with matcher-rules (default: no matcher)")
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
      log.Print(err)
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
        log.Printf("prevent panic by handling failure accessing a path %q: %v\n", dir, err2)
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
		  log.Printf("error walking the path %q: %v\n", dir, err2)
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
          log.Printf("%v %v", v[8], uevent.Env["DEVNAME"])
          shellport, shellporterr := checkPort("4300","4201")
          if shellporterr != nil {
            log.Println(shellporterr)
          }
          //log.Println(shellport)
          nginxport, nginxporterr := checkPort("8300","8201")
          //log.Println(nginxport)
          if nginxporterr != nil {
            log.Println(nginxporterr)
          }
          if nginxporterr == nil && shellporterr == nil {
            /* no errors, do the thing */
            starterr := startShellinabox(v[8], uevent.Env["DEVNAME"], shellport)
            if starterr == nil {
              nginxconferr := createNginxConf(v[8], nginxport, shellport)
              if nginxconferr == nil {
                _ = reloadNginx()
              }
            }
          }
        } else {
          allRecords = remove(allRecords, Record{UdevId: v[8], DeviceId: "/dev/"+uevent.Env["DEVNAME"]})
          log.Printf("%v %v", v[8], uevent.Env["DEVNAME"])
          stoperr := stopShellinabox(v[8])
          if stoperr == nil {
            removeconferr := removeNginxConfig(v[8])
            if removeconferr == nil {
              _ = reloadNginx()
            }
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

func startShellinabox(udev string, devname string, shellport string) error {
  /* start shellinabox localhost */
  shellcmd := exec.Command("/usr/bin/shellinaboxd","-t","-s","/:nobody:nogroup:/:screen -D -R -S "+udev+" /dev/"+devname+" 9600 -o","-p",shellport,"--background=/app/loomis/run/"+udev+".pid","--localhost-only")
  shellerr := shellcmd.Start()
  shellcmd.Wait()
  if shellerr != nil {
    log.Println(shellerr)
    return shellerr
  }
  return nil
  /* start shellinabox */
}

func stopShellinabox(udev string) error {
  /* kill shellinabox */
  pidnum, _ := exec.Command("cat", "/app/loomis/run/"+udev+".pid").CombinedOutput()
  pidcmd := exec.Command("kill","-9",string(pidnum))
  piderr := pidcmd.Start()
  pidcmd.Wait()
  if piderr != nil {
    log.Println(piderr)
    return piderr
  }
  piderr = os.Remove("/app/loomis/run/"+udev+".pid")
  if piderr != nil {
    log.Println(piderr.Error())
    return piderr
  }
  return nil
  /* kill shellinabox */
}

func reloadNginx() error {
  /* reload nginx */
  nginxcmd := exec.Command("service","nginx","reload")
  nginxerr := nginxcmd.Start()
  nginxcmd.Wait()
  if nginxerr != nil {
    log.Println(nginxerr)
    return nginxerr
  }
  return nil
  /* reload nginx */
}

func createNginxConf(udev string, nginx_port string, shell_port string) error {
  /* create nginx template */
  nginx_uuid := uuid.New().String()
  c := templatevars{
    NginxPort: nginx_port,
    ShellPort: shell_port,
    NginxUuid: nginx_uuid,
  }
  t, err := template.ParseFiles("/app/loomis/nginx-template.conf.tpl")
  if err != nil {
    log.Print(err)
    return err
  }
  f, err := os.Create("/app/loomis/config/"+udev+".conf")
  if err != nil {
    log.Println("create file: ", err)
    return err
  }
  err = t.Execute(f, c)
  if err != nil {
    log.Print("execute: ", err)
    return err
  }
  f.Close()
  return nil
  /* create nginx template */
}

func removeNginxConfig(udev string) error {
  /* remove nginx config */
  conferr := os.Remove("/app/loomis/config/"+udev+".conf")
  if conferr != nil {
    log.Println(conferr.Error())
    return conferr
  }
  return nil
  /* remove nginx config */
}

func checkPort(porthigh string, portlow string) (string, error) {
  /* check port in range is unused */
  porta, _ := strconv.Atoi(portlow)
  portb, _ := strconv.Atoi(porthigh)
  port := ""
  for i := porta; i <= portb; i++ {
    port = strconv.Itoa(i)
    ln, err := net.Listen("tcp", ":" + port)
    if err != nil {
      log.Printf("Can't listen on port %q: %s\n", port, err)
      continue
    } else {
      _ = ln.Close()
      log.Printf("TCP Port %q is available\n", port)
      return port, nil
    }
  }
  return "", errors.New("Unable to allocate port")
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
