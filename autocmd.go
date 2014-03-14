package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/howeyc/fsnotify"
)

var whitespace = regexp.MustCompile("\\s")

func watch(patterns []*regexp.Regexp, n string) bool {
	for _, reg := range patterns {
		if reg.MatchString(n) {
			return true
		}
	}
	return false
}

const (
	not = iota
	some
	yeah
	very
	extremely
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	dir := flag.String("dir", wd, "What directory to watch")
	ignore := flag.String("ignore", "\\/\\.", "What files and directories to always ignore")
	verbose := flag.Int("verbose", 0, "How verbose to be, higher is more verbose")
	wait := flag.Int("wait", 1000, "Milliseconds to wait before running cmd, in case changes happen in clusters")
	between := flag.Int("between", 0, "Milliseconds to wait before starting proces again after a stop")
	sigint := flag.Int("sigint", 0, "If set, process will be killed nicely and got this many ms before kill")

	oldUsage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "./autocmd <flags> <command> -- <files_to_monitor>\n")
		oldUsage()
	}

	flag.Parse()

	cmds := []string{}
	for _, c := range flag.Args() {
		if c == "--" {
			break
		}
		cmds = append(cmds, c)
	}
	if len(cmds) < 1 {
		flag.Usage()
		return
	}

	patterns := []*regexp.Regexp{}

	for _, pattern := range flag.Args() {
		patterns = append(patterns, regexp.MustCompile(pattern))
	}

	ignorePattern := regexp.MustCompile(*ignore)

	if len(patterns) < 1 {
		flag.Usage()
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	restart := make(chan bool)

	// Process events
	go func() {
		var waiting int32
		part := ""
		for {
			select {

			case ev := <-watcher.Event:
				part = ev.Name[len(*dir):]
				if !ignorePattern.MatchString(part) {
					if watch(patterns, part) {
						atomic.AddInt32(&waiting, 1)
						go func() {
							if *verbose > yeah {
								log.Printf("File changed: %#v\n", ev)
							}
							time.Sleep(time.Millisecond * time.Duration(*wait))
							if atomic.AddInt32(&waiting, -1) == 0 {
								if *verbose > yeah {
									log.Printf("Restart needed: %v\n", ev)
								}
								restart <- true
							}
						}()
					}
					if ev.IsCreate() {
						if st, err := os.Stat(ev.Name); err != nil {
							fmt.Println(err)
						} else if st.IsDir() || watch(patterns, part) {
							if *verbose > yeah {
								log.Printf("%v: Watching %#v\n", ev, ev.Name)
							}
							if err = watcher.Watch(ev.Name); err != nil {
								fmt.Println(err)
							}
						}
					}
				}
			case err := <-watcher.Error:
				fmt.Println(err)
			}
		}
	}()

	queue := []string{*dir}
	next := ""
	part := ""
	for len(queue) > 0 {
		next = queue[0]
		part = next[len(*dir):]
		queue = queue[1:]
		st, err := os.Stat(next)
		if err != nil {
			panic(err)
		}
		if !ignorePattern.MatchString(part) {
			if st.IsDir() || watch(patterns, part) {
				if *verbose > yeah {
					fmt.Println("Watching", part, ignorePattern)
				}
				err = watcher.Watch(next)
				if err != nil {
					panic(err)
				}
			}
			if st.IsDir() {
				f, err := os.Open(next)
				if err != nil {
					panic(err)
				}
				subs, err := f.Readdir(-1)
				if err != nil {
					panic(err)
				}
				if err := f.Close(); err != nil {
					panic(err)
				}
				for _, sub := range subs {
					if sub.IsDir() {
						queue = append(queue, filepath.Join(next, sub.Name()))
					}
				}
			}
		}
	}

	go func() {
		for {
			command := exec.Command(cmds[0], cmds[1:]...)
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			if err := command.Start(); err != nil {
				fmt.Println(err)
			}
			if *verbose > not {
				log.Printf("Running %v pid: %v\n", cmds, command.Process.Pid)
			}

			<-restart
			if *verbose > some {
				log.Printf("Killing %v pid: %v\n", command.Path, command.Process.Pid)
			}

			if *sigint > 0 {
				if err := command.Process.Signal(syscall.SIGINT); err != nil {
					log.Printf("Unable to sigint process: %s pid: %v\n", err, command.Process.Pid)
				}
				time.Sleep(time.Millisecond * time.Duration(*sigint))
			}

			if err := command.Process.Kill(); err != nil {
				log.Printf("Unable to kill process: %s pid: %v\n", err, command.Process.Pid)
			}

			if err := command.Wait(); err != nil {
				log.Printf("Process pid: %v exited with: %s\n", command.Process.Pid, err)
			}
			time.Sleep(time.Millisecond * time.Duration(*between))
		}
	}()

	if *verbose > some {
		log.Printf("Watching %#v matching %v, running %#v on changes\n", *dir, patterns, cmds)
	}
	x := make(chan bool)
	<-x

}
