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
  "time"
  "bytes"
  "regexp"

  "github.com/pilebones/go-udev/netlink"
  "github.com/gorilla/mux"
  "github.com/google/uuid"
  "github.com/tarm/serial"

)

var (
	filePath *string
)

var consoles = []byte(`[]`)
var stateFile = ""

var greensKeeper = ""
var greensKeeperToken = ""
var loomisServer = ""
var httpPort = ""
var consolesPort = ""
var dockerConsolesPort = ""
var dockerHttpPort = ""

type Exception struct {
  Message string `json:"message"`
}

type ConsoleRecordList map[string]ConsoleRecord

type ConsoleRecord struct {
  UdevId   string `json:"udev"`
  DeviceId string `json:"device"`
  ShellPort string `json:"port"`
  NginxUuid string `json:"uuid"`
  BaudRate string `json:"baud"`
  Status string `json:"status"`
  Permissions consolePermList `json:"permissions"`
}

type permInfo struct {
  UserName        string `json:"username"`
  PassWord      string `json:"password"`
}

type templatevars struct {
	NginxPort string
	ShellPort string
	NginxUuid string
}

var ConsoleRecords ConsoleRecordList

type consoleList map[string]ConsoleInfo

type ConsoleInfo struct {
  Id             string    `json:"id"`
  UdevPath        string  `json:"console_udev"`
  PathConsole        string  `json:"console_path"`
  BaudConsole        string  `json:"console_baudrate"`
  ServerConsole        string  `json:"console_server"`
}

type consolePermList map[string]consolePerm

type consolePerm struct {
  Id        string `json:"slot_id"`
  //SlotId        string `json:"slot_id"`
  UserName      string `json:"username"`
  PassWord      string `json:"password"`
}

func init() {
	filePath = flag.String("file", "", "Optionnal input file path with matcher-rules (default: no matcher)")
}

func main() {
  greensKeeper = os.Getenv("GK_SERVER")
  greensKeeperToken = os.Getenv("GK_TOKEN")
  httpPort = os.Getenv("HTTP_PORT")
  consolesPort = os.Getenv("CONSOLES_PORT")
  dockerHttpPort = os.Getenv("DOCKER_HTTP_PORT")
  dockerConsolesPort = os.Getenv("DOCKER_CONSOLES_PORT")
  loomisServer = os.Getenv("LOOMIS_SERVER")
  ConsoleRecords = make(ConsoleRecordList)

  if greensKeeper == "" {
    log.Fatalln("GK_SERVER env var not set")
  }
  if greensKeeperToken == "" {
    log.Fatalln("GK_TOKEN env var not set")
  }
  if loomisServer == "" {
    log.Fatalln("GK_TOKEN env var not set")
  }
  if httpPort == "" {
    httpPort = "8080"
    //log.Fatalln("HTTP_PORT env var not set")
  }
  if consolesPort == "" {
    consolesPort = "8081"
    //log.Fatalln("HTTP_PORT env var not set")
  }
  if dockerHttpPort == "" {
    dockerHttpPort = httpPort
    //log.Fatalln("HTTP_PORT env var not set")
  }
  if dockerConsolesPort == "" {
    dockerConsolesPort = dockerConsolesPort
    //log.Fatalln("HTTP_PORT env var not set")
  }

	flag.Parse()
  stateFile = os.Getenv("STATE_FILE")
  if stateFile == "" {
    stateFile = "/app/loomis/config/output.json"
    //log.Fatalln("STATE_FILE env var not set")
  }
  result, err := ioutil.ReadFile(stateFile) // just pass the file name
  if err != nil {
      log.Print(err)
  }
  consoles = result
  if err := json.Unmarshal([]byte(consoles), &ConsoleRecords); err != nil {
    log.Println(err)
  }
  /* gather what we know already from the records and check they exist on the system */
  dir := "/sys/devices" //all usb devices will be in here somewhere, lets find them
  /*
   start checking for any devices that arent in the statefile
   they may have been added between a service restart
  */
  files, err := filepath.Glob("/dev/ttyUSB*")
  if err != nil {
      log.Fatal(err)
  }
  for _, f := range files {
    v := strings.Split(f, "/dev/")
    fmt.Println(v[1])
    b := recordContains(ConsoleRecords,v[1])
    fmt.Println(b)
    found := b
    err2 := filepath.Walk(dir, func(path string, info os.FileInfo, err2 error) error {
      if err2 != nil {
        log.Printf("prevent panic by handling failure accessing a path %q: %v\n", dir, err2)
        return err2
      }
      if found != true {
        if strings.Contains(path, v[1]) {
          splitpath := strings.Split(path, "/")
          log.Printf("path %v", splitpath[len(splitpath)-2])
          found = true
          nginx_uuid := uuid.New().String()
          udevid := splitpath[len(splitpath)-2]
          ConsoleRecords[udevid] = ConsoleRecord{
            UdevId: udevid,
            DeviceId: v[1],
            ShellPort: "4201",
            NginxUuid: nginx_uuid,
            BaudRate: "9600",
            Status: "disconnected",
          }
          addNewRecord(ConsoleRecords)
          return nil
        }
      }
      return nil
    })
    if err2 != nil {
		  log.Printf("error walking the path %q: %v\n", dir, err2)
	  }
  }
  var jsonBytes []byte
  jsonBytes, err = json.Marshal(ConsoleRecords)
  fmt.Println(string(jsonBytes))
  /* end check */
  /*
   check statefile matches what devices we have plugged in already,
   and start or remove as required
  */
  for i := range ConsoleRecords {
    found := false
    err2 := filepath.Walk(dir, func(path string, info os.FileInfo, err2 error) error {
      if err2 != nil {
        log.Printf("prevent panic by handling failure accessing a path %q: %v\n", dir, err2)
        return err2
      }
      if found != true {
        if strings.Contains(path, ConsoleRecords[i].UdevId) {
          if strings.Contains(info.Name(), ConsoleRecords[i].DeviceId) {
            log.Printf("Device exists and is active; udev:%v,  devname: %v", ConsoleRecords[i].UdevId, ConsoleRecords[i].DeviceId)
            found = true
            return nil
            /* it exists */
          }
        }
      }
      return nil
    })
    /* if we don't find a match at all, then remove it from the records
      we can check for running services using this device and kill them here too
    */
    if found == false {
      updateGreensKeeper(ConsoleRecords[i].UdevId, ConsoleRecords[i].NginxUuid, false, "", "")
      //allRecords = append(allRecords[:i], allRecords[i+1:]...)
      delete(ConsoleRecords, ConsoleRecords[i].UdevId)
      deleteHtpass(ConsoleRecords[i].UdevId)
    } else {
      if ConsoleRecords[i].Status == "connected" {
        starterr := startShellinabox(ConsoleRecords[i].UdevId, ConsoleRecords[i].DeviceId, ConsoleRecords[i].ShellPort, ConsoleRecords[i].BaudRate)
        if starterr != nil {
          log.Println(starterr)
        }
      } else {
        log.Printf("Device is not connected to any slots, shellinabox not started; udev:%v,  devname: %v", ConsoleRecords[i].UdevId, ConsoleRecords[i].DeviceId)
      }
    }
    if err2 != nil {
		  log.Printf("error walking the path %q: %v\n", dir, err2)
	  }
  }
  /* end check */
  result, jsonerr := json.Marshal(ConsoleRecords)
  if jsonerr != nil {
    log.Println(jsonerr)
  }
  //log.Printf("Result: %v", string(result))
  err = ioutil.WriteFile(stateFile, result, 0644)
  nginxconferr := createNginxConf(ConsoleRecords)
  if nginxconferr == nil {
    _ = reloadNginx()
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
	go monitor(matcher, ConsoleRecords) //run udev nonblocking?

  /* api */
  r := mux.NewRouter()
  r.HandleFunc("/api/v1/consoles", ListConsoles).Methods("GET")
  log.Println("Ready to serve consoles!")
  port, _ := strconv.Atoi(httpPort)
  log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", port), r))
}

func monitor(matcher netlink.Matcher, ConsoleRecords ConsoleRecordList) {
	log.Println("Monitoring UEvent kernel message to user-space...")

	conn := new(netlink.UEventConn)
	if err := conn.Connect(netlink.KernelEvent); err != nil {
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
          //kill read after 30 seconds of no bytes
          nginx_uuid := uuid.New().String()

          serial, macaddress, sererr := getSerial(uevent.Env["DEVNAME"], "115200", time.Second * 45, v[len(v)-4])
          if sererr != nil {
            if serial != "" {
              fmt.Printf("Serial No: %v\n", serial[2:])
              serial = strings.ToLower(serial[2:])
            } else {
              fmt.Printf("MAC Address: %v\n", macaddress)
              macaddress = strings.ToLower(macaddress)
              if macaddress != "" {
                macsplit := strings.Split(macaddress, ":")
                serial = macsplit[3]+macsplit[4]+macsplit[5]
              }
            }
          }

          consoleData, conerr := getConsoleFromGreensKeeper(v[len(v)-4])
          if len(consoleData) == 1 && conerr == nil {
            /* no errors, do the thing */
            for i := range consoleData {
              consoleUsers, usererror := getPermissionsFromGreensKeeper(consoleData[i].Id)
              if usererror != nil {
              }
              baudRate := consoleData[i].BaudConsole
              ConsoleRecords[v[len(v)-4]] = ConsoleRecord{
                UdevId: v[len(v)-4],
                DeviceId: uevent.Env["DEVNAME"],
                ShellPort: "4201",
                NginxUuid: nginx_uuid,
                BaudRate: baudRate,
                Status: "connected",
                Permissions: consoleUsers,
              }
              addNewRecord(ConsoleRecords)
              log.Printf("Inserted device; udev: %v, devname: %v", v[len(v)-4], uevent.Env["DEVNAME"])
              starterr := startShellinabox(v[len(v)-4], uevent.Env["DEVNAME"], "4201", baudRate)
              if starterr == nil {
                nginxconferr := createNginxConf(ConsoleRecords)
                if nginxconferr == nil {
                  _ = reloadNginx()
                  updateGreensKeeper(v[len(v)-4], nginx_uuid, true, serial, macaddress)
                }
              }
            }
          } else {
            /* add console in disconnected state */
            ConsoleRecords[v[len(v)-4]] = ConsoleRecord{
              UdevId: v[len(v)-4],
              DeviceId: uevent.Env["DEVNAME"],
              ShellPort: "4201",
              NginxUuid: nginx_uuid,
              BaudRate: "9600",
              Status: "disconnected",
            }
            addNewRecord(ConsoleRecords)
          }
        } else {
          log.Printf("Removed device; udev: %v, devname: %v", v[len(v)-4], uevent.Env["DEVNAME"])
          removeRecord := ConsoleRecords[v[len(v)-4]]
          delete(ConsoleRecords, v[len(v)-4])
          deleteHtpass(v[len(v)-4])
          stoperr := stopShellinabox(v[len(v)-4])
          if stoperr == nil {
            removeconferr := createNginxConf(ConsoleRecords)
            if removeconferr == nil {
              _ = reloadNginx()
              updateGreensKeeper(removeRecord.UdevId, removeRecord.NginxUuid, false, "", "")
            }
          }
        }
        result, err := json.Marshal(ConsoleRecords)
        if err != nil {
          log.Println(err)
        }
        //log.Printf("Result: %v", string(result))
        consoles = result
        err = ioutil.WriteFile(stateFile, result, 0644)
      }
		case err := <-errors:
			log.Printf("ERROR: %v", err)
		}
	}
}

func recordContains(arr ConsoleRecordList, str string) bool {
   for a := range arr {
      if arr[a].DeviceId == str {
         return true
      }
   }
   return false
}

func getSerial(device string, baud string, timeout time.Duration, udevid string) (string, string, error) {
  baudrate, _ := strconv.Atoi(baud)
  log.Println(device)
  log.Println(baud)
  log.Println(udevid)
  c := &serial.Config{Name: "/dev/"+device, Baud: baudrate, ReadTimeout: timeout}
  s, err := serial.OpenPort(c)
  //defer s.Close()
  if err != nil {
    fmt.Println(err)
    return "", "", err
  }
  //fmt.Println("insert detected /dev/"+device)
  buf := make([]byte, 40)
  var content []byte
  count := 0
  for {
    n, err := s.Read(buf)
    if err != nil {
      //need to fix this so it stops spewing "EOF" to screen
      //fmt.Println(err)
    }

    if n == 0 {
      log.Println("No data from serial device")
      break
      //return "", errors.New("Nothing received from serial")
    }
    content = append(content, buf[:n]...)
    log.Println(content)
    login_pattern := regexp.MustCompile("login:")
    login_match := login_pattern.FindString(strings.TrimSpace(string(content)))
    if len(login_match) > 1 {
      // we got a login prompt!
      log.Println("Got login prompt from serial device")
      //fmt.Printf("%v\n", login_match)
      //fmt.Println(string(content))
      break
    }
    count++
  }
  // FIXME maybe we should dump the log somewhere
  //fmt.Println(string(content))
  writeerr := ioutil.WriteFile("/app/loomis/config/"+udevid+".log", content, 0644)
  if writeerr != nil {
    return "", "", nil
  }

  serial_pattern := regexp.MustCompile("serial=(0[xX]).{8,8}")
  serial_match := strings.Split(serial_pattern.FindString(strings.TrimSpace(string(content))), "=")
  mac_pattern := regexp.MustCompile("([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})")
  mac_match := mac_pattern.FindString(strings.TrimSpace(string(content)))
  serial_number := ""
  mac_address := ""
  if len(serial_match) > 1 {
    // we got a serial number! strip first 2 characters from it
    fmt.Printf("%v\n", serial_match[1][2:])
    //s.Close()
    serial_number = string(serial_match[1][2:])
  }
  if len(mac_match) > 1 {
    // we got a mac address!
    fmt.Printf("%v\n", mac_match)
    //s.Close()
    mac_address = string(mac_match)
  }

  s.Close()
  return serial_number, mac_address, errors.New("Nothing received from serial")
}

func startShellinabox(udev string, devname string, shellport string, baudrate string) error {
  /* start shellinabox localhost */
  newshellport, shellporterr := checkPort("4300","4201")
  if shellporterr != nil {
    log.Println(shellporterr)
    return shellporterr
  }
  updateRecordShellPort(udev, newshellport)
  shellcmd := exec.Command("/usr/bin/shellinaboxd","-t","-s","/:nobody:nogroup:/:screen -D -R -S "+udev+" /dev/"+devname+" "+baudrate+" -o","-p",newshellport,"--background=/app/loomis/run/"+udev+".pid","--localhost-only")
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

func deleteHtpass(udevId string) {
  delhtpass := os.Remove("/app/loomis/config/"+udevId+".htpass")
  if delhtpass != nil {
    log.Println(delhtpass.Error())
  }
}

func updateRecord(udevId string, status string, baudrate string) string {
  if ConsoleRecords[udevId].UdevId == udevId {
    deviceId := ConsoleRecords[udevId].DeviceId
    nginxUuid := ConsoleRecords[udevId].NginxUuid
    shellport, shellporterr := checkPort("4300","4201")
    if shellporterr != nil {
      log.Println(shellporterr)
      return "Error finding free port"
    }
    /* remove record, then add record */
    delete(ConsoleRecords, udevId)
    deleteHtpass(udevId)
    stoperr := stopShellinabox(udevId)
    if stoperr == nil {
      removeconferr := createNginxConf(ConsoleRecords)
      if removeconferr == nil {
        _ = reloadNginx()
      }
    }
    ConsoleRecords[udevId] = ConsoleRecord{
      UdevId: udevId,
      DeviceId: deviceId,
      ShellPort: shellport,
      NginxUuid: nginxUuid,
      BaudRate: baudrate,
      Status: status,
    }
    if status == "connected" {
      starterr := startShellinabox(udevId, deviceId, shellport, baudrate)
      if starterr != nil {
        log.Println(starterr)
        return "Error starting console"
      } else {
        nginxconferr := createNginxConf(ConsoleRecords)
        if nginxconferr == nil {
          _ = reloadNginx()
        }
        return "No matches"
      }
    }
  }
  return "No matches"
}

func addNewRecord(ConsoleRecords ConsoleRecordList) {
  result, jsonerr := json.Marshal(ConsoleRecords)
  if jsonerr != nil {
    log.Println(jsonerr)
  }
  err := ioutil.WriteFile(stateFile, result, 0644)
  if err != nil {
		log.Fatalln(err.Error())
	}
}

func updateRecordShellPort(udevId string, shellport string) string {
  if ConsoleRecords[udevId].UdevId == udevId {
    deviceId := ConsoleRecords[udevId].DeviceId
    nginxuuid := ConsoleRecords[udevId].NginxUuid
    baudrate := ConsoleRecords[udevId].BaudRate
    status := ConsoleRecords[udevId].Status
    /* remove record, then add record */
    delete(ConsoleRecords, udevId)
    consoleData, _ := getConsoleFromGreensKeeper(udevId)
    consoleUsers := make(consolePermList)
    if len(consoleData) == 1 {
      for i := range consoleData {
        consoleUsers, _ = getPermissionsFromGreensKeeper(consoleData[i].Id)
      }
    }
    ConsoleRecords[udevId] = ConsoleRecord{
      UdevId: udevId,
      DeviceId: deviceId,
      ShellPort: shellport,
      NginxUuid: nginxuuid,
      BaudRate: baudrate,
      Status: status,
      Permissions: consoleUsers,
    }
    addNewRecord(ConsoleRecords)
  }
  return "No matches"
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

func createNginxConf(c ConsoleRecordList) error {
  /* create nginx template */
  t, err := template.ParseFiles("/app/loomis/nginx-template.conf.tpl")
  if err != nil {
    log.Print(err)
    return err
  }
  ht, err := template.ParseFiles("/app/loomis/htpass.tpl")
  if err != nil {
    log.Print(err)
    return err
  }
  for d := range c {
    log.Printf("create file: /app/loomis/config/%v.htpass", c[d].UdevId)
    fht, err := os.Create("/app/loomis/config/"+c[d].UdevId+".htpass")
    if err != nil {
      log.Println("create file: ", err)
      return err
    }
    m := map[string]interface{}{
      "Users": c[d].Permissions,
    }
    err = ht.Execute(fht, m)
    if err != nil {
      log.Print("execute: ", err)
      return err
    }
    fht.Close()
  }
  //f, err := os.Create("/app/loomis/config/"+udev+".conf")
  f, err := os.Create("/app/loomis/config/loomis.conf")
  if err != nil {
    log.Println("create file: ", err)
    return err
  }
  m := map[string]interface{}{
    "Consoles": c,
    "NginxPort": consolesPort,
  }
  err = t.Execute(f, m)
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
      //log.Printf("Can't listen on port %q: %s\n", port, err)
      continue
    } else {
      _ = ln.Close()
      //log.Printf("TCP Port %q is available\n", port)
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

func PostConsole(w http.ResponseWriter, req *http.Request) {
  w.Write(consoles)
}

func getConsoleFromGreensKeeper(UdevId string) (consoleList, error) {
  var netClient = &http.Client{
    Timeout: time.Second * 10,
  }
  token := greensKeeperToken
  req, _ := http.NewRequest("GET", greensKeeper+"/api/v1/console/"+UdevId, nil)
  //req.Header.Add("Authorization", "Bearer "+token)
  req.Header.Add("apikey", token)
  resp, _ := netClient.Do(req)
  //log.Printf("%v%v%v", greensKeeper,"/api/v1/console/",UdevId)
  defer resp.Body.Close()
  body, _ := ioutil.ReadAll(resp.Body)
  textBytes := []byte(body)
  //log.Printf(string(textBytes))
  list := make(consoleList)
  if err := json.Unmarshal([]byte(textBytes), &list); err != nil {
    log.Println(err)
  }
  if len(list) == 1 {
    for i := range list {
      slotId := list[i].UdevPath
      if string(slotId) == "" {
        log.Printf("No matching identifier for %v", slotId)
        return list, errors.New("No matching identifier for")
      } else {
        log.Printf("Found %v",slotId)
        return list, nil
      }
    }
  } else {
    log.Printf("No matching identifier for %v", UdevId)
    return list, errors.New("No matching identifier for")
  }
  return list, errors.New("No matching identifier for")
}

func getPermissionsFromGreensKeeper(slotId string) (consolePermList, error) {
  var netClient = &http.Client{
    Timeout: time.Second * 10,
  }
  token := greensKeeperToken
  req, _ := http.NewRequest("GET", greensKeeper+"/api/v1/console/perms/"+slotId, nil)
  //req.Header.Add("Authorization", "Bearer "+token)
  req.Header.Add("apikey", token)
  resp, _ := netClient.Do(req)
  //log.Printf("%v%v%v", greensKeeper,"/api/v1/console/perms/",slotId)
  defer resp.Body.Close()
  body, _ := ioutil.ReadAll(resp.Body)
  textBytes := []byte(body)
  //log.Printf(string(textBytes))
  list := make(consolePermList)
  if err := json.Unmarshal([]byte(textBytes), &list); err != nil {
    log.Println(err)
  }
  return list, nil
}

func updateGreensKeeper(udevid string, nginxuuid string, server bool, associated_pi string, macaddress string) {
  consoleData, conerr := getConsoleFromGreensKeeper(udevid)
  //log.Printf(string(udevid))
  //if nginxporterr == nil && shellporterr == nil {
  if len(consoleData) == 1 && conerr == nil {
    /* no errors, do the thing */
    for i := range consoleData {
      slotId := consoleData[i].Id
      var netClient = &http.Client{
        Timeout: time.Second * 10,
      }
      token := greensKeeperToken
      var jsonStr = []byte("")
      if server == true {
        if associated_pi != "" && macaddress != "" {
          jsonStr = []byte(`{"consoleserver": "`+loomisServer+`:`+dockerConsolesPort+`", "consolepath": "`+nginxuuid+`", "associated_pi": "`+associated_pi+`", "macaddress":"`+macaddress+`"}`)
        } else if associated_pi != "" && macaddress == "" {
          jsonStr = []byte(`{"consoleserver": "`+loomisServer+`:`+dockerConsolesPort+`", "consolepath": "`+nginxuuid+`", "associated_pi": "`+associated_pi+`"}`)
        } else if associated_pi == "" && macaddress != "" {
          jsonStr = []byte(`{"consoleserver": "`+loomisServer+`:`+dockerConsolesPort+`", "consolepath": "`+nginxuuid+`", "macaddress":"`+macaddress+`"}`)
        } else {
          jsonStr = []byte(`{"consoleserver": "`+loomisServer+`:`+dockerConsolesPort+`", "consolepath": "`+nginxuuid+`"}`)
        }
      } else {
        jsonStr = []byte(`{"consoleserver": "undefined", "consolepath": "undefined"}`)
      }
      req, _ := http.NewRequest("POST", greensKeeper+"/api/v1/slots/"+string(slotId), bytes.NewBuffer(jsonStr))
      //req.Header.Add("Authorization", "Bearer "+token)
      req.Header.Add("apikey", token)
      req.Header.Set("Content-Type", "application/json")
      resp, _ := netClient.Do(req)
      //body, _ := ioutil.ReadAll(resp.Body)
      //log.Printf(string(body))
      defer resp.Body.Close()
    }
  }
}
