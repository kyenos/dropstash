// +build linux

package main

/*-----------------------------------------------
 monitor.go

 Contains core logic for dropstash that relates
 to monitoring a location for new files
-----------------------------------------------*/
import (
	"code.google.com/p/go-uuid/uuid"
	log "github.com/Sirupsen/logrus"
	"golang.org/x/exp/inotify"
	"os"
	"path"
)

/* This is where we do the actual location monitoring. This
   is started as a concurent go routine, and monitors loc_id
   indefinatly.
   - loc_id is the index to the config.Locations array to
     monitor*/
func monitor(loc_id int, cont chan bool) {
	log.Println("Spinning up monitor on location ID:", loc_id)

	stop := false
	location := config.Locations[loc_id]
	watcher, err := inotify.NewWatcher()
	if err != nil {
		log.Error(err)
		return
	}
	err = watcher.Watch(location)
	if err != nil {
		log.Error(err)
		return
	}
	log.Infoln("Watcher up; monitoring:", location)
	for !stop {
		cached_id := uuid.New() //cache a new uuid
		select {
		case ev := <-watcher.Event:
			log.Debugln("monitored directory event:", ev)
			if ev.Mask == inotify.IN_CLOSE_WRITE {
				log.Info("Found; ", path.Base(ev.Name), " Moving to staging")
				os.Rename(ev.Name, config.Staging_loc+"/"+cached_id)
				var op Operation
				op.Code = ProcessFile
				op.Id = cached_id
				op.Name = path.Base(ev.Name)
				op.Location = path.Dir(ev.Name)
				op.Overwrite = false //TODO determine if this should be gleamed from the file name
				meta.stash <- op
			}
		case err := <-watcher.Error:
			log.Error("Monitor error;", err)
			break
		case stop = <-cont:
			log.Infoln("Spinning down monitor on ", location)
			break
		}
	}
}
