// +build linux

/* Dropstash

   Currently only runs on linux! */

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/sevlyar/go-daemon"
)

var (
	config       Config
	meta         Meta
	signal       *string
	cmd_args     []string
	invalid      = "invalid"
	as_daemon    = flag.Bool("d", false, `when combined with start, run as a system daemon`)
	debug        = flag.Bool("debug", false, `Turn on debug level logging`)
	mon_notifier = make(chan bool)
)

//go:generate /bin/bash -c "./build_dependencies.sh"
func main() {

	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	if len(flag.Args()) > 0 {
		signal = &flag.Args()[0]
		cmd_args = flag.Args()[1:]
	} else { //ugly hack since we cant point to an anonymous const?
		signal = &invalid
	}

	if match, err := regexp.MatchString("start|stop|reload|status|remove|list|export", *signal); !match || err != nil {
		log.Errorln("Must provide at at least one command (start, stop, reload, status, remove, list, export)")
		fmt.Println("Optional flags:")
		flag.PrintDefaults()
		os.Exit(1)
	} //*/

	if *signal != "start" && *as_daemon {
		log.Errorln("Can't run as daemon unless operation is start")
		flag.PrintDefaults()
		os.Exit(1)
	}
	config.LoadConfig()
	if len(config.Locations) < 1 && *signal == "start" {
		log.Fatal("Must have one or more locatoins to monitor. Please edit config file")
	}

	daemon.AddCommand(daemon.StringFlag(signal, "stop"), syscall.SIGTERM, termHandler)
	daemon.AddCommand(daemon.StringFlag(signal, "reload"), syscall.SIGHUP, reloadHandler)
	daemon.SetSigHandler(termHandler, syscall.SIGINT)

	cntxt := &daemon.Context{
		PidFileName: (config.Config_loc + "/pid"),
		PidFilePerm: 0640,
		LogFileName: (config.Log_loc + "/current"),
		LogFilePerm: 0640,
		WorkDir:     config.Config_loc,
		Umask:       027,
		Args:        []string{"[dropstash-daemon]", "start"},
	}

	//this is where we process quit, stop, reload
	if len(daemon.ActiveFlags()) > 0 {
		d, err := cntxt.Search()
		if err != nil {
			log.Fatalln("Unable send signal to the daemon:", err)
		}
		daemon.SendCommands(d)
		return
	}
	switch {
	case *signal == "status":
		cmd := "/bin/ps -ef | /bin/egrep 'dropstash |dropstash\\-daemon' | /bin/egrep -v 'grep|status'"
		out, err := exec.Command("/bin/sh", "-c", cmd).Output()
		if err != nil {
			fmt.Printf("\nDropstash daemon not running, please start.\n\n")
		} else {
			fmt.Printf("\nDropstash running:\n%s\n", out)
		}
	case *signal == "list":
		meta.LoadStashFile()
		const layout = "Jan 02 06 15:04:23"
		for _, node := range meta.Files {
			for _, file := range node.Pointers {
				nm := file.Name
				if len(file.Name) > 30 {
					nm = file.Name[:27] + "..."
				}
				np := file.Location
				if len(file.Location) > 35 {
					np = file.Location[:32] + "..."
				}
				fmt.Printf("%-36s %-30s %10d %3d %-40s %v\n",
					node.Id, nm, file.Size, file.Version, np, file.VersionDate.Format(layout))
			}
		}
	case *signal == "export":
		meta.LoadStashFile()
		log.Debugln("Length of args is:", len(cmd_args))
		if len(cmd_args) != 2 {
			log.Fatalln("Copy requires both a source and a destination")
			os.Exit(1)
		}
		node, file, _ := meta.Lookup(cmd_args[0])
		log.Debugln("node:\n", node, "\nFile:\n", file)
		log.Debugln("output file:", cmd_args[1])
		meta.ExportFile(*node, *file, cmd_args[1])

	case *signal == "remove":
		meta.LoadStashFile()
		log.Debugln("Length of args is:", len(cmd_args))
		for itr := 0; itr < len(cmd_args); itr++ {
			meta.RemoveFile(cmd_args[itr])
		}
	case *signal == "start":

		if *as_daemon { //if we flaged daemon, we do our fork
			d, err := cntxt.Reborn()
			if err != nil {
				log.Fatalln(err)
			}
			if d != nil {
				return //we are outta here
			}
		} else {
			err := cntxt.NoBackground()
			if err != nil {
				log.Fatal("Failed no background,", err)
			}
		}
		defer cntxt.Release()
		log.Infoln("- - - - - - - - - - - - - - -")
		log.Infoln("daemon started")
		config.LoadConfig() //must reload since we are in a child process
		log.Infoln("Loaded config")
		meta.init()

		go meta.OpenStash()
		for itr := 0; itr < len(config.Locations); itr++ {
			go monitor(itr, mon_notifier)
		} //*/

		err := daemon.ServeSignals()
		if err != nil {
			log.Println("Error:", err)
		}
		log.Infoln("Daemon terminated")
	}
}

func termHandler(sig os.Signal) error {

	log.Println("Cleaning up...")
	if sig == syscall.SIGTERM ||
		sig == syscall.SIGINT {
		stopOp := new(Operation)
		stopOp.Code = Stop
		meta.stash <- *stopOp
	}
	<-meta.stash

	return daemon.ErrStop
}

func reloadHandler(sig os.Signal) error {
	for range config.Locations {
		mon_notifier <- true
	}
	config.LoadConfig()
	for itr := 0; itr < len(config.Locations); itr++ {
		go monitor(itr, mon_notifier)
	} //*/
	log.Println("configuration reloaded")
	return nil
}
