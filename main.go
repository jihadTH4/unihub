package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const appPort = 47821

var isWindows = runtime.GOOS == "windows"

func main() {
	background := flag.Bool("background", false, "run without opening the browser")
	port := flag.Int("port", appPort, "local port")
	waitPort := flag.Bool("wait-port", false, "wait for the previous instance to release the port (after update/install)")
	updatedFrom := flag.String("updated-from", "", "previous version, set by the updater")
	flag.Parse()

	if err := initDataDir(); err != nil {
		fmt.Println("cannot create data folder:", err)
		os.Exit(1)
	}
	lf, err := os.OpenFile(filepath.Join(dataDir, "unihub.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		if fi, _ := lf.Stat(); fi != nil && fi.Size() > 5<<20 {
			lf.Truncate(0)
		}
		log.SetOutput(io.MultiWriter(os.Stderr, lf))
	}
	url := "http://127.0.0.1:" + strconv.Itoa(*port) + "/"

	ln, err := listen(*port)
	for i := 0; err != nil && *waitPort && i < 100; i++ {
		time.Sleep(200 * time.Millisecond)
		ln, err = listen(*port)
	}
	if err != nil {
		// already running? just open the window
		c := &http.Client{Timeout: 2 * time.Second}
		if resp, err2 := c.Get(url + "api/ping"); err2 == nil {
			resp.Body.Close()
			if !*background {
				openURL(url)
			}
			return
		}
		log.Fatal("cannot listen: ", err)
	}

	loadConfig()
	loadState()
	loadFeedback()
	indexer.Load()
	afterUpdate(*updatedFrom)
	go indexer.loop()
	go scheduler()
	go feedbackLoop()
	go updateLoop()

	log.Println("UniHub", Version, "running at", url)
	if !*background {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openURL(url)
		}()
	}
	httpSrv = &http.Server{Handler: newServer(*port), ReadHeaderTimeout: 10 * time.Second}
	if err := httpSrv.Serve(ln); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	select {} // restarting: wait for os.Exit
}

var httpSrv *http.Server
