// +build linux

package main

/*-----------------------------------------------
 config.go

 Represents a set of configuration options for
 dropstash

-----------------------------------------------*/
import (
	//"encoding/json"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"time"

	log "github.com/Sirupsen/logrus"
)

/* Config represents the global configuration options available
   to dropstash. At the moment these are:
   - The locations for the daemon to monitor
   - The location for the daemon to log to
   - The log roll-over period in days
   - The location of the stash
   - The location of the configuration directory
   - The working directory root for the daemon (also config_loc)
   The configuration file is read only so to reload values you
   must restart the daemon. This also makes it very thread safe.*/
type Config struct {
	Locations          []string
	Log_loc            string
	Log_roll           int
	Stash_loc          string
	Stash_save_seconds time.Duration
	Config_loc         string
	Staging_loc        string
}

/* LoadConfig initializes the ~/.dropstash location and it's
   config file. The file is simple JSON that directly matches
   the Config structure. The following applies:
   - if ~/.dropstash doesn't exist, create it
   - if ~/.dropstash/config doesn't exist create it and
     populate it with resonable defaults
   - if ~/.dropstash/config exists, load it into the global
     config variable. */
func (self *Config) LoadConfig() {

	//get the current user information
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	//resonable defaults
	self.Log_loc = usr.HomeDir + "/.dropstash/logs"
	self.Log_roll = 1
	self.Stash_save_seconds = 30
	self.Stash_loc = usr.HomeDir + "/.dropstash/stash"
	self.Staging_loc = usr.HomeDir + "/.dropstash/stash/staging"
	confDir := usr.HomeDir + "/.dropstash"
	self.Config_loc = confDir
	self.Locations = nil

	//check for ~/.dropstash
	if _, err := os.Stat(confDir); os.IsNotExist(err) {
		log.Warnf("No existing .dropstash: %s, creating", confDir)
		os.MkdirAll(confDir, 0700)
	}

	//check for ~/.dropstash/config, write it if not there
	if _, err := os.Stat(confDir + "/config"); err != nil {
		fl, err := os.Create(confDir + "/config")
		if err != nil {
			log.Fatal(err)
		}
		defer fl.Close()
		st, err := json.MarshalIndent(&self, "", "    ")
		fmt.Fprintf(fl, "%s", st)
		log.Println("Created config")
	} else { //otherwise load the config file into memory
		fl, err := os.Open(confDir + "/config")
		if err != nil {
			log.Fatal(err)
		}
		defer fl.Close()
		err = json.NewDecoder(fl).Decode(&self)
		if err != nil {
			log.Error("Error parsing config file.")
			log.Fatal(err)
		}
	}

	//ok got config, check for log location, make if not there
	if _, err := os.Stat(self.Log_loc); err != nil {
		err := os.MkdirAll(self.Log_loc, 0700)
		if err != nil {
			log.Fatal("Couldn't create log directory.")
		}
	}
}
