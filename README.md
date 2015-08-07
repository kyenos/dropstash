#dropstash

I wanted to learn how to program in go, so I took on a task at the request of a good friend of mine and applied my new knowledge to it.

Dropstash is a Linux daemon capable of concurrently monitoring an arbitrary number of file locations, with guaranteed access to any written file immediately after it's been closed by the writing author. This guarantee comes from the kernel through the inotify subsystem of the Linux kernel, which at this time, is the only monitoring subsystem dropstash is capable of. That might change in the future depending on whether or not there is any real practical need for dropstash in the field, and if that need requires it to run on alternative platforms.

OK, so why monitor an arbitrary number of random directories? The answer is simple... the idea is, that those locations are public or, semi public locations for users / authors to drop stuff they don't want others to see. Let me give you a real world example.

My friend has several clients that want to drop file backups somewhere. The system those backups end up on, has a shared user account for those clients. When one of them drops a file into the directory, dropstash whisks it away into the stash. Now only the drop stash user account can see the file.

This could be somewhat problematic of an idea for several reasons:
what if :
- Somebody gets mad and starts dumping hundreds of thousands of versions of the same file to fill up the space?
- Somebody gets smart, and dumps many files that are similar or the same inside but, with different names?
- Somebody doesn't transfer the whole file, and only a portion gets dumped?

There are many potential security as well as practical issues that need to be addressed, and that's exactly what dropstash does. It doesn't keep files, it keeps a stash of bytes, in the longest consecutive identical series it can. It then keeps a metadata database on those byte stashes, and exactly what files are in there. So, for example, if somebody drops 100 exact versions of a dvd.iso that's exactly 1.7G with 100 different names, and 75 of them came through partially because of network failures, well, then dropstash will keep only 1.7G in the stash, and details with pointers and verification meta data on the 100 different attempts, all of which can be recovered identically as transmitted from the stash.

Additionally, since dropstash is being run as a different user, not just anybody can get at the contents of the files in the stash.

So, dropstash does solve a real world dilemma, in an efficient way but the real question is, will any body use it (or even really need it)?

##Build and dependencies:

Dropstash currently only works on Linux... not UNI* but Linux. The reason is that the inotify subsystem, as far as I know at the moment, is a Linux kernel feature. If enough people request that it runs on other systems, I'll consider making it more portable 

Dropstash has some external dependencies as well as requires a patched version of a go package called go-daemon. These dependencies are automatically installed and compiled for you.

So how to get started...

go get github.com/kyenos/dropstash
```
cd $GOPATH/src/github.com/kyenos/dropstash

go generate

go build

./dropstash [-d] <start, stop, status, reload, list, export, remove>
```

Optionally, if your $GOPATH/bin is in path for your system; go install will publish it there.

##Application and arguments

dropstash consists of a single, statically compiled binary that can be run off the command line. It operates in 1 of 2 modes:
- deamon;     It's acting as the drop monitor and stash daemon (optionally as a real deamon in the background)
- management; Where one inspects, removes or exports file from the cache.

Arguments:
```
 -d                  Run daemon in background (only available with start)
 start               Start in daemon mode
 stop                Stop any given running daemon 
 status              Determine if dropstash is running
 reload              Reload a running dropstash's config file (located in ~/.dropstash)
 list                List files in the stash
 export              Export a file from the stash (return it to it's original condition)
 remove              Remove a stash or file from the system. Removing a stash takes all
                     the related files with it.
```                     
On first start, dropstash will create the stash and configuration files in ~/.dropstash. It will then warn you that you haven't supplied anywhere for it to monitor so it will exit. Edit the ~/.dropstash/config file it should like something like this:

```
 {
    "Locations": [ ],
    "Log_loc": "/home/kyenos/.dropstash/logs",
    "Log_roll": 1,
    "Stash_loc": "/home/kyenos/.dropstash/stash",
    "Stash_save_seconds": 10,
    "Config_loc": "/home/kyenos/.dropstash"
}
```
You must edit the Locations array, here is an example:
```
{
    "Locations": ["/home/kyenos/tmp/m1",
                  "/home/kyenos/tmp",
                  "/home/kyenos/tmp/m2" ],
    "Log_loc": "/home/kyenos/.dropstash/logs",
    "Log_roll": 1,
    "Stash_loc": "/home/kyenos/.dropstash/stash",
    "Stash_save_seconds": 10,
    "Config_loc": "/home/kyenos/.dropstash"
}
```

##Feature list and status.

Check out [Features.txt]((https://github.com/kyenos/dropstash/blob/master/Features.txt) for details on the status of individual feature. This will be updated when things change when future features are added to the utility

For feature requests or bugs, please create an issue and I'll try to address it in a timely manor.

Good luck, have fun!

