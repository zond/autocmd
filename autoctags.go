package main

import (
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var whitespace = regexp.MustCompile("\\s")

func main() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	dir := flag.String("dir", wd, "What directory to watch")
	cmd := flag.String("cmd", "ctags --recurse --sort=yes", "What command to run on changes")
	ignore := flag.String("ignore", "^/tags$,/\\.,~$", "Comma separated list of regexpes to ignore")
	verbose := flag.Bool("verbose", false, "Whether to verbosely explain what happens")
	wait := flag.Int("wait", 1000, "Milliseconds to wait before running cmd, in case changes happen in clusters")

	flag.Parse()

	ignores := []*regexp.Regexp{}

	for _, ign := range strings.Split(*ignore, ",") {
		ignores = append(ignores, regexp.MustCompile(ign))
	}

	parts := whitespace.Split(*cmd, -1)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	// Process events
	go func() {
		ignore := false
		part := ""
		var waiting int32
		for {
			select {
			case ev := <-watcher.Event:
				ignore = false
				part = ev.Name[len(*dir):]
				for _, reg := range ignores {
					if reg.MatchString(part) {
						ignore = true
						break
					}
				}
				if !ignore {
					atomic.AddInt32(&waiting, 1)
					go func() {
						<-time.After(time.Millisecond * time.Duration(*wait))
						if atomic.AddInt32(&waiting, -1) == 0 {
							if *verbose {
								fmt.Printf("%v: Running %#v\n", ev, *cmd)
							}
							command := exec.Command(parts[0], parts[1:]...)
							command.Stdout = os.Stdout
							command.Stderr = os.Stderr
							if err := command.Run(); err != nil {
								fmt.Println(err)
							}
						}
					}()
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
		if *verbose {
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
			if sub.IsDir() && strings.Index(sub.Name(), ".") != 0 {
				queue = append(queue, filepath.Join(next, sub.Name()))
			}
		}
	}

	if *verbose {
		fmt.Printf("Watching %#v, ignoring %v, running %#v on changes\n", *dir, ignores, *cmd)
	}
	x := make(chan bool)
	<-x

}