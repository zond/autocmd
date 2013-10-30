package main

import (
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"
)

var whitespace = regexp.MustCompile("\\s")

func watch(patterns []*regexp.Regexp, n string) bool {
	for _, reg := range patterns {
		if reg.MatchString(n) {
			return true
		} else {
			fmt.Println("wth, ", n, "doesnt match", reg)
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
	cmd := flag.String("cmd", "", "What command to run on changes")
	verbose := flag.Int("verbose", 0, "How verbose to be, higher is more verbose")
	wait := flag.Int("wait", 1000, "Milliseconds to wait before running cmd, in case changes happen in clusters")

	oldUsage := flag.Usage
	flag.Usage = func() {
		oldUsage()
		fmt.Fprintf(os.Stderr, "All extra arguments are parsed as regular expressions to watch within -dir\n")
	}

	flag.Parse()

	if *cmd == "" || len(flag.Args()) == 0 {
		flag.Usage()
		return
	}

	patterns := []*regexp.Regexp{}

	for _, pattern := range flag.Args() {
		patterns = append(patterns, regexp.MustCompile(pattern))
	}

	parts := whitespace.Split(*cmd, -1)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	var command *exec.Cmd

	// Process events
	go func() {
		var waiting int32
		for {
			select {
			case ev := <-watcher.Event:
				if watch(patterns, ev.Name[len(*dir):]) {
					atomic.AddInt32(&waiting, 1)
					go func() {
						<-time.After(time.Millisecond * time.Duration(*wait))
						if atomic.AddInt32(&waiting, -1) == 0 {
							if command != nil {
								if *verbose > some {
									fmt.Printf("%v: Killing %v", ev, command.Path)
								}
								if err := command.Process.Kill(); err != nil {
									panic(err)
								}
							}
							if *verbose > not {
								fmt.Printf("%v: Running %#v\n", ev, *cmd)
							}
							command = exec.Command(parts[0], parts[1:]...)
							command.Stdout = os.Stdout
							command.Stderr = os.Stderr
							if err := command.Start(); err != nil {
								fmt.Println(err)
							}
						}
					}()
					if ev.IsCreate() {
						if f, err := os.Open(ev.Name); err != nil {
							fmt.Println(err)
						} else {
							if *verbose > yeah {
								fmt.Printf("%v: Watching %#v\n", ev, ev.Name)
							}
							if err = watcher.Watch(ev.Name); err != nil {
								fmt.Println(err)
							}
							f.Close()
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
	for len(queue) > 0 {
		next = queue[0]
		queue = queue[1:]
		if *verbose > yeah {
			fmt.Println("Watching", next)
		}
		err := watcher.Watch(next)
		if err != nil {
			panic(err)
		}
		f, err := os.Open(next)
		if err != nil {
			panic(err)
		}
		subs, err := f.Readdir(-1)
		if err != nil {
			panic(err)
		}
		for _, sub := range subs {
			if sub.IsDir() {
				queue = append(queue, filepath.Join(next, sub.Name()))
			}
		}
	}

	if *verbose > not {
		fmt.Printf("Running %#v\n", *cmd)
	}
	command = exec.Command(parts[0], parts[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		panic(err)
	}

	if *verbose > some {
		fmt.Printf("Watching %#v matching %v, running %#v on changes\n", *dir, patterns, *cmd)
	}
	x := make(chan bool)
	<-x

}
