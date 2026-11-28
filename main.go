package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const appPort = 47821

var isWindows = runtime.GOOS == "windows"

func main() {
	background := flag.Bool("background", false, "run without opening the browser")
	port := flag.Int("port", appPort, "local port")
	waitPort := flag.Bool("wait-port", false, "wait for the previous instance to release the port (after update/install)")
	updatedFrom := flag.String("updated-from", "", "previous version, set by the updater")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(Version)
		return
	}

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
		// Another copy is already running. If it is older than this one, replace it;
		// otherwise just open its window.
		running, ok := pingRunning(url)
		if !ok {
			log.Fatal("cannot listen: ", err)
		}
		if !newerVersion(Version, running) {
			if !*background {
				openURL(url)
			}
			return
		}
		log.Println("taking over from running version", running)
		if err := stopRunning(url); err != nil {
			log.Println("handoff:", err)
			openURL(url)
			return
		}
		if dst, moved := replaceInstalledCopy(); moved {
			startDetached(dst, "--wait-port", "--updated-from="+running)
			return
		}
		for i := 0; i < 100 && err != nil; i++ {
			time.Sleep(200 * time.Millisecond)
			ln, err = listen(*port)
		}
		if err != nil {
			log.Fatal("cannot listen: ", err)
		}
		if *updatedFrom == "" {
			*updatedFrom = running
		}
	}

	// remember which version ran last, so an update installed from outside (the Android
	// installer, or a new exe copied over the old one) still shows "what's new"
	vf := filepath.Join(dataDir, "last-version")
	if b, err := os.ReadFile(vf); err == nil {
		if prev := strings.TrimSpace(string(b)); *updatedFrom == "" && prev != "" && newerVersion(Version, prev) {
			*updatedFrom = prev
		}
	}
	os.WriteFile(vf, []byte(Version), 0o644)

	loadConfig()
	loadState()
	loadFeedback()
	indexer.Load()
	afterUpdate(*updatedFrom)
	go indexer.loop()
	go scheduler()
	go feedbackLoop()
	go updateLoop()
	go videoWorker()
	go func() {
		// bring other copies of UniHub.exe on this computer up to this version
		time.Sleep(20 * time.Second)
		syncCopies()
	}()
	go func() {
		// the Android app stops the engine with SIGTERM: save everything before leaving
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		indexer.Save()
		saveState()
		saveFeedback()
		os.Exit(0)
	}()

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
