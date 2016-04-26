// +build linux

package main

/*-----------------------------------------------
 meta.go

 Represents live meta data of the current stash

-----------------------------------------------*/
import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

/* A node contains the bytes of a file in the stash, a file
   represents a portion of said Node, and therefore a single
   Node can represent multiple versions of the file sent in.
   For example, a user transfers part of a file but the net
   cuts it off, the subsequent transfer will keep the whole
   file (a single node) but the stash sees both transfers as
   two file entries (or version) in the Node.

   A second example, the file gets modified i.e. truncated an
   is now newer but a smaller portion of the same file. The
   Node keeps the original but a pointer to the 'new' file

   In this way we optamize the bytes stored in the stash and
   keep all versions of any given file transfered until removed
   from the stash. A Node remains in the stash until all File
   pointers are exhausted, then frees the longest portion remaining.*/
type FilePointer struct {
	Name        string
	Location    string
	Size        int64
	VersionDate time.Time
	Version     int
}

/* Interface used to compare File Pointers to each other */
func (self *FilePointer) Compare(to *FilePointer) (ret bool) {
	ret = false
	if self.Name == to.Name &&
		self.Location == to.Location &&
		self.Size == to.Size &&
		self.VersionDate == to.VersionDate &&
		self.Version == to.Version {
		ret = true
	}
	return
}

/* Represents a fast hash lookup for any given file in the stash.

   Based on a lookup string node.Id/filename[:version]

   for example; 30b55313-1f3b-4f9e-bcab-cfd2c74e7c75/a:0
   will find file a version 0 in stash 30b55313-1f3b-4f9e-bcab-cfd2c74e7c75

   Future working on this might dissambiguate by Location as well but
   as it stands now, one will always be able to identify by requesting a list */
type LookupPointer struct {
	file *FilePointer
	node *Node
}

/* Represents a file in the stash. The monitor thread will
   maintain this struct, the array of them as well as the
   actual meta file in the stash location.
   - Names is an array representing all the different file names
     this file has been associated with.
   - Id is the UUID, a uniqe identifier within the stash for this
     file, regarless of name. The combination of the UUID and the
	 check sum value can be used to remove duplicate files from
	 stash
   - ChkSum is the MD5 sumation of the contents of the file. This
     produces a unique has for the x number of bytes available in
	 the file. The combination of x number of bytes, the ChkSum
	 can be used to determine if other files are a partial duplcate
	 within the stash. Only the largest x number of bytes wins out
	 on the duplication front and is actually kept in the stash
   - Size is the number of bytes available in the file on disk.
   - PickupCount is used to determine the number of times this file
     has been picked up from a monitoring location.
   - PartilCount is the number of times this file was picked up with
     less than the number of bytes in the longest chain.
   - Overwrite is a method for the monitor to eliminate a Node and
     replace it with the new file... not currently implemented
   - MaxSize, is the largest FilePointer.Size in Names, allowing
     optamization and truncation if bytes are removed from a Node.
   In order for partial processing to be accurate, files must be marked
   as being transfered with overwrite if the sender intends to send an
   identical file with less bytes. */
type Node struct {
	Pointers     []FilePointer
	Id           string
	ChkSum       string
	Size         int64
	PickupCount  int
	PartialCount int
	Overwrite    bool
}

/* Interface used to compare File Pointers to each other */
func (self *Node) Compare(to *Node) (ret bool) {
	ret = false
	if self.Id == to.Id &&
		self.ChkSum == to.ChkSum {

		ret = true
	}
	return
}

/* OpCode and Operation struct are a combination used to send messages
   to the Meta stash.
   - ProcessFile contains the file to process,
   - start and stop are the control structures for the channel
   - Use go generate to generate the opcode_string.go file*/
//go:generate stringer -type=OpCode
type OpCode int

const (
	Start OpCode = iota
	ProcessFile
	Stop
	Pause
)

/* Operation passed along the channel to the stash. This is used for
   communication between the monitor and the stash thread */
type Operation struct {
	Code      OpCode
	Name      string
	Location  string
	Id        string
	Overwrite bool
}

/* The Meta struct contains the actual stash metadata:
   - Files contains a list of individual file objects and their details
   - Count is a cash of the total number of files that should be in the
    array. This is a convience value for readability within the file and
	therefore is printed first within the file
   The global var stash is used by the meta channel to maintain the live
   stash */
type Meta struct {
	Count    int
	Files    []Node
	pointers map[string]map[int]LookupPointer
	stash    chan Operation
}

/* Initialize our Meta object. This is necessary because we need the
   channel up front */
func (self *Meta) init() {
	log.Info("Initializing the stash channel")
	self.pointers = make(map[string]map[int]LookupPointer)
	self.stash = make(chan Operation) //make our channel, must be first

}

func (self *Meta) LoadStashFile() {
	//load the meta data
	fl, err := os.OpenFile(config.Config_loc+"/meta", os.O_RDONLY|os.O_CREATE, 0600)
	if err != nil {
		log.Warn(err)
	}
	err = json.NewDecoder(fl).Decode(self)
	if err != nil {
		log.Warn("Error parsing meta file: ", err)
	}
	fl.Close() //we keep going even on failure so we must close
	self.RebuildLookup()
}

/* Underlying meta management is done on the Meta object. Open stash
   implements the inital load of the data responsible for the stash.
   timed saves are made to the data store when run in daemon mode. */
func (self *Meta) OpenStash() {

	log.Info("Opening stash")
	//check for ~/.dropstash
	if _, err := os.Stat(config.Staging_loc); os.IsNotExist(err) {
		log.Warnf("No existing stash: %s, creating", config.Stash_loc)
		log.Warnf("No existing staging location: %s, creating", config.Staging_loc)
		os.MkdirAll(config.Staging_loc, 0700)
	}

	self.LoadStashFile()
	log.Println("Loaded available meta data")
	defer close(self.stash) //defer some cleanup
	curr_op := Operation{}  //get the next operation... better be start

	for curr_op.Code != Stop {
		log.Debugln("Processing opcodes and stash save")
		select { //get the next operation, and occasionally save the stash

		case <-time.After(config.Stash_save_seconds * time.Second):
			self.SaveStash()
		case curr_op = <-self.stash:
			log.Debugln("Processing next Operation:", curr_op.Code)
			if curr_op.Code == ProcessFile {
				fl, err := os.Open(config.Staging_loc + "/" + curr_op.Id)
				if err != nil {
					log.Errorln("Failed to open staging file:", curr_op.Id)
					continue
				}
				var file Node
				file.Id = curr_op.Id
				file.Overwrite = curr_op.Overwrite
				if fd, err := fl.Stat(); err != nil {
					log.Errorln("Failed to get stats on staged file:", file.Id)
					continue
				} else {
					file.Size = fd.Size()
				}
				file.ChkSum, err = self.calcMd5sum(fl, file.Size)
				file.PickupCount = 1
				file.PartialCount = 0
				pointer := FilePointer{curr_op.Name, curr_op.Location, file.Size, time.Now(), 0}
				file.Pointers = append(file.Pointers, pointer)
				//Now that we have a 'current file', we can append it to the stash
				self.append(file, fl) //Note that fl is closed in append
			}
		}
	}
	self.SaveStash() //make sure we clean up
	meta.stash <- curr_op
}

/* Calculate the md5 sum for reader x up to n bytes
   This is used to produce compareable results */
func (self *Meta) calcMd5sum(in io.Reader, bc int64) (ret string, err error) {
	hash := md5.New()
	buff := make([]byte, 4096)
	var soFar int64
	stop := false
	log.Debugln("incoming md5 request, ", bc, " bytes")
	for {
		sz, err := in.Read(buff)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if err == io.EOF || stop {
			log.Debugln("EOF at ", bc, " bytes")
			break
		}

		if (soFar + int64(sz)) > bc {
			log.Debugln("So Far (", soFar, ") is greater than ", bc, " bytes if we add ", sz, " bytes")
			sz -= int((soFar + int64(sz)) - bc)
			log.Debugln("Adjusted sz to ", sz, " bytes")
			stop = true
		}
		soFar += int64(sz)

		log.Debugln("hashing ", sz, " bytes")
		_, err = hash.Write(buff[:sz])
		if err != nil {
			log.Errorln("Failed during md5 sum read")
			break
		}
	}
	if err == nil {
		bts := hash.Sum(nil)
		ret = fmt.Sprintf("%x", bts)
	}
	log.Debugln("logged and hashed ", soFar, " bytes")
	return
}

/* De-duplicate staging / stash note this should be private to
   Meta */
func (self *Meta) append(stgNode Node, stgFile *os.File) {

	pointer := stgNode.Pointers[0] //there can only be one here!
	log.Debugln("A dump of our file so far:\n***\n %v\n\n***", stgNode)
	defer self.RebuildLookup() //we can do this nomatter what the outcome
	defer self.SaveStash()
	for itr := range self.Files { //loop over everything in the stash if we have to
		node := &self.Files[itr]
		log.Debugln("Comparing to:", node.Id)
		if node.ChkSum == stgNode.ChkSum { //we have a flat out duplicate
			log.Info("Found a duplicate of ", node.Id)
			pointer.Version = len(node.Pointers)
			node.Pointers = append(node.Pointers, pointer)
			node.PickupCount += 1
			stgFile.Close()
			os.Remove(config.Staging_loc + "/" + stgNode.Id)
			return
		}
		stashfl, err := os.Open(config.Stash_loc + "/" + node.Id)
		if err != nil {
			log.Errorln("Failed to open stash file: ", node.Id)
			break //TODO; is it possible that this could introduce a zombi?
		}
		leftCheck, _ := self.calcMd5sum(stashfl, stgNode.Size)
		stashfl.Close()
		log.Debugln("LeftCheck for stgNode.size; ", stgNode.Size, " is: ", leftCheck)
		if leftCheck == stgNode.ChkSum { //incoming file is a partial of this file
			log.Info("Incoming file is a partial of: ", node.Id)
			pointer.Version = len(node.Pointers)
			node.Pointers = append(node.Pointers, pointer)
			node.PartialCount += 1
			stgFile.Close() //we only add the pointer and remove the staged file
			os.Remove(config.Staging_loc + "/" + stgNode.Id)
			return
		}
		stgFile.Seek(0, 0)
		rightCheck, _ := self.calcMd5sum(stgFile, node.Size)
		log.Debugln("RightCheck for stgNode.size; ", stgNode.Size, " is: ", rightCheck)
		if rightCheck == node.ChkSum { //stashed file is a partial of the incoming file
			log.Info("Stashed file ", node.Id, " is a partial of incoming file")
			pointer.Version = len(node.Pointers)
			node.Pointers = append(node.Pointers, pointer)
			node.PickupCount += 1
			node.PartialCount += 1
			stgFile.Close() //we keep the incoming file and ditch the staged file, keep the old id
			os.Rename(config.Staging_loc+"/"+stgNode.Id, config.Stash_loc+"/"+node.Id)
			node.Size = stgNode.Size
			node.ChkSum = stgNode.ChkSum
			return
		}
	} //stage file is unique to the stash, add and move
	log.Info("New file is unique, adding to stash as", stgNode.Id)
	stgFile.Close()
	self.Files = append(self.Files, stgNode) // this happens if we are not a duplicate or partial
	self.Count = len(self.Files)
	os.Rename(config.Staging_loc+"/"+stgNode.Id, config.Stash_loc+"/"+stgNode.Id)
	return
}

/* Save the current state of the stash. This will happen periodically
   regardless of whether or not anything happens in the watched locations
   - Use config.Stash_save_seconds to determine how long
*/
func (self *Meta) SaveStash() {
	fl, err := os.Create(config.Config_loc + "/meta")
	if err != nil {
		log.Fatal(err)
	}
	defer fl.Close()
	st, err := json.MarshalIndent(&self, "", "    ")
	if err != nil {
		log.Errorln("Failed to open our meta data file", err)
	}
	_, err = fmt.Fprintf(fl, "%s", st)
	if err != nil {
		log.Errorln("Failed to write meta data", err)
	}
	log.Debugln("Saved meta data")
}

/* Looks over and rebuilds the lookup table.
   This is actually very inefficient however it
   sure beats rolling through arrays for each
   lookup by a long shot, this does it n1 for stash
   change*/
func (self *Meta) RebuildLookup() {

	self.pointers = make(map[string]map[int]LookupPointer)
	for _, node := range self.Files {
		fpn := self.pointers[node.Id]
		var lkn LookupPointer
		lkn.node = &node
		lkn.file = nil
		if fpn == nil {
			fpn = map[int]LookupPointer{}
		}
		fpn[0] = lkn
		self.pointers[node.Id] = fpn
		//for itr := 0; itr < len(node.Pointers); itr++ {
		for _, file := range node.Pointers {
			fps := self.pointers[node.Id+"/"+file.Name]
			var lkp LookupPointer
			lkp.file = new(FilePointer)
			lkp.node = new(Node)
			*lkp.file = file
			*lkp.node = node
			if fps == nil {
				fps = map[int]LookupPointer{}
			}
			fps[file.Version] = lkp
			self.pointers[node.Id+"/"+file.Name] = fps
		}
	}
	log.Debugln(self.pointers)

}

/* Do a lookup on the stash for a give stash node... format:
   {stashid}/filename<:version>
   returns nil node / file  for not found*/
func (self *Meta) Lookup(stash_node string) (node *Node, file *FilePointer, exact bool) {

	base_name := stash_node
	version := 0
	exact = false
	if idx := strings.Index(stash_node, ":"); idx > 0 {
		base_name = stash_node[:idx]
		vi64, _ := strconv.ParseInt(stash_node[(idx+1):], 10, 64)
		version = int(vi64)
	}
	log.Debugln("Basename is: ", base_name, "Version is: ", version)
	if fls := self.pointers[base_name]; fls != nil {
		var found LookupPointer
		log.Debugln("Found: ", fls, " from ", base_name, " for ", version)
		if v, ok := fls[version]; ok || (len(fls) == 1 && version == 0) {
			if v.node == nil && v.file == nil {
				for _, fv := range fls {
					v = fv
					break
				}
			}
			log.Debug("Found: ", v, " for version: ", version)
			found = v
			exact = true
		} else {
			for x := range fls { //find the lowest version
				kv := 1000 //skip whole stash only
				log.Debugln("Kv level: ", fls[x].file)
				if fls[x].file != nil && fls[x].file.Version < kv {
					found = fls[x]
					kv = found.file.Version
				}
			}
		}
		file = found.file
		log.Debugln("File is: ", found.file)
		node = found.node
		log.Debugln("Node is: ", node)
	}
	return
}

/* Remove a version, file or stash from the stash format:
   {stashid}/<filename><:version>
   will remove everything if asked */
func (self *Meta) RemoveFile(stash_node string) {

	node, file, exact := meta.Lookup(stash_node)
	log.Debugln("\n\n*** \nFound: ", file, "\n", exact, "\n***\n\n")
	if node != nil {
		if file != nil {
			if exact {
				log.Println("Removing file: ", file.Name, " version: ", file.Version, " from stash: ", node.Id)
				self.pullFromFiles(node, file)
				return
			} else {
				log.Println("Didn't find exact file, should I remove version:", file.Version, " [yes/No]")
				if Ask("no") {
					self.pullFromFiles(node, file)
				}
				return
			}
			return
		} else {
			log.Println("Asked to remove entire stash... are you sure? [yes/No]")
			if Ask("no") {
				self.pullFromFiles(node, nil)
				os.Remove(config.Stash_loc + "/" + node.Id)
			}
			return
		}
	}
	log.Println("Unable to find file to remove")
	return
}

/* Used by Remove file, this rebuilds the splice and assigns
   the new array of Files to self */
func (self *Meta) pullFromFiles(node *Node, file *FilePointer) {

	whole_stash := false
	if file == nil {
		whole_stash = true
	}
	var new_files []Node
	for _, itr := range self.Files {
		if itr.Compare(node) && whole_stash {
			log.Debugln("skipping whole stash: ", itr.Id)
			continue
		} else if itr.Compare(node) && !whole_stash {
			var new_pointers []FilePointer
			for _, fitr := range itr.Pointers {
				if fitr.Compare(file) {
					log.Debugln("skipping file from stash")
					continue
				}
				new_pointers = append(new_pointers, fitr)
			}
			itr.Pointers = new_pointers
		}
		new_files = append(new_files, itr)
	}
	self.Files = new_files
	log.Debug("\n\n***\nFiles:\n\n", self.Files, "\n\n***\n\n")
	self.SaveStash()
}

/* Export a file from the stash somewhere... if the somewhere is a
   directory, we tack on file's name, if it's a file we export to
   the new file name */
func (self *Meta) ExportFile(node Node, file FilePointer, loc string) {

	log.Debugln("Opening stash: ", node.Id)
	fl, err := os.Open(config.Stash_loc + "/" + node.Id)
	st, err := fl.Stat()
	if err != nil || st.IsDir() {
		log.Errorln("Invalid stash, failed to open:", node.Id)
		return
	}
	defer fl.Close()

	log.Debugln("Testing for directory?")
	if st, err := os.Stat(loc); err == nil { //adjusts if loc is a dir
		if st.IsDir() {
			loc += "/" + file.Name
		}
	}

	log.Debugln("Opening output file: ", loc)
	of, err := os.OpenFile(loc, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Errorln("Failed to open output location:", loc)
		return
	}
	defer of.Close()

	sp := file.Size
	wrote := 0
	buff := make([]byte, 4096)

	log.Debugln("This is the file object:\n", file, "\n")
	log.Debugln("Scanning input file for ", file.Size, " bytes")
	for {
		sz, err := fl.Read(buff)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if err == io.EOF || sp <= 0 {
			log.Debugln("EOF at ", sz, " bytes")
			break
		}
		if int64(sz) > file.Size {
			sz = int(file.Size)
		}
		wr, err := of.Write(buff[:sz])
		//log.Debugln("Read ", sz, " bytes, Wrote ", wr, " bytes to output file")
		if err != nil {
			log.Errorln("Failure during file export from stash:", loc)
		}
		sp -= int64(wr)
		wrote += wr
		log.Debugln(sp, " bytes left to write")
	}
	log.Debugln("Wrote: ", wrote, " bytes total")
}

/* Ask the user if this is OK */
func Ask(def string) bool {
	var response string
	fmt.Scanln(&response) //no error checking here, anything is OK

	ok, _ := regexp.Compile(`(?i)y`) //no error checking here, the regex is always the same
	nok, _ := regexp.Compile(`(?i)n`)

	if len(response) < 1 {
		response = def
	}
	if ok.MatchString(response) {
		return true
	} else if nok.MatchString(response) {
		return false
	} else {
		fmt.Println("Please type yes or no and then press enter:")
		return Ask(def)
	}
}
