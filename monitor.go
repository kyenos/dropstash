// +build linux

package main

/*-----------------------------------------------
 monitor.go

 Contains core logic for dropstash that relates
 to monitoring a location for new files
-----------------------------------------------*/
import (
	"errors"
	"os"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

/* Simple check for permissions, ensures user is in the
   correct group. */
func checkPermissions(location string) (loc_info os.FileInfo, err error) {

	defer func() {
		if err != nil {
			log.Errorln("Make sure location has:\n\t- File group set umask\n\t- In a group for this user\n\t- Is a directory")
		}
	}()
	loc_info, err = os.Stat(location)
	if err != nil {
		return
	}

	if !loc_info.Mode().IsDir() {
		err = errors.New("Location mode must be a directory.")
		return
	}
	if loc_info.Mode()&os.ModeSetgid == 0 {
		err = errors.New(location + "\n\t does not have the correct permissions to be monitored: " + loc_info.Mode().String())
		return
	}
	return
}

/* This is where we do the actual location monitoring. This
   is started as a concurent go routine, and monitors loc_id
   indefinatly.
   - loc_id is the index to the config.Locations array to
     monitor*/
func monitor(loc_id int, cont chan bool) {
	log.Println("Spinning up monitor on location ID:", loc_id)

	stop := false
	location := config.Locations[loc_id]
	if st, err := checkPermissions(location); err != nil {
		log.Error(err)
		return
	} else {
		log.Infoln("Permissions check for", location, "passed:", st.Mode())
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error(err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(location)
	if err != nil {
		log.Error(err)
		return
	}
	log.Infoln("Watcher up; monitoring:", location)
	for !stop {
		cached_id := uuid.New().String() //cache a new uuid
		select {
		case ev := <-watcher.Events:
			log.Debugln("monitored directory event:", ev)
			if ev.Op&fsnotify.Write == fsnotify.Write {
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
		case err := <-watcher.Errors:
			log.Error("Monitor error;", err)
			continue
		case stop = <-cont:
			log.Infoln("Spinning down monitor on ", location)
			break
		}
	}
}
