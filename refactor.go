package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Refactor struct {
	sync.Mutex
	Matches  []Match
	Uniques  []string
	Oldtext  string
	Newtext  string
	Filelist []string
}

type Match struct {
	Filename   string
	LineText   string
	GrepFormat string
	LineNumber int
}

func main() {
	flag.Usage = func() {
		fmt.Println("Refactor")
		fmt.Println()
		fmt.Println("Searches all the files in the current directory containing")
		fmt.Println("the [old] and replacement every occurrence with [new]. Be ")
		fmt.Println("aware that forward slashes must be escaped in both cases.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  refactor [old-text] [new-text]")
		fmt.Println("  refactor [old-text] [new-text] [files]")
		os.Exit(2)
	}

	flag.Parse()

	app := NewRefactor(flag.Arg(0), flag.Arg(1), flag.Args())

	if err := app.Execute(); err != nil {
		fmt.Println(err.Error())
	}
}

func NewRefactor(oldtext string, newtext string, filelist []string) *Refactor {
	return &Refactor{
		Oldtext:  oldtext,
		Newtext:  newtext,
		Filelist: filelist,
	}
}

func (r *Refactor) Execute() error {
	if r.Oldtext == r.Newtext {
		return errors.New("Old and new text are the same")
	}

	if len(r.Filelist) == 2 {
		r.Filelist = r.FindFiles()
	}

	/* discard the first two elemnets in the list */
	r.Filelist = append([]string{}, r.Filelist[2:]...)

	r.GrepDirectory()

	if len(r.Matches) == 0 {
		return errors.New("Nothing to refactor")
	}

	r.PrintMatches()

	r.ReplaceMatches()

	return nil
}

func (r *Refactor) FindFiles() []string {
	filelist := []string{".", ".."}
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if path[0] == '.' {
			return nil
		}
		filelist = append(filelist, path)
		return nil
	})
	return filelist
}

func (r *Refactor) GrepDirectory() {
	var wg sync.WaitGroup
	wg.Add(len(r.Filelist))
	for _, filename := range r.Filelist {
		go r.InspectFile(&wg, filename)
	}
	wg.Wait()
}

func (r *Refactor) InspectFile(wg *sync.WaitGroup, filename string) {
	file, err := os.Open(filename)

	if err != nil {
		fmt.Println(filename, err)
		return
	}

	defer func() {
		file.Close()
		wg.Done()
	}()

	var counter int
	var content string

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		content = scanner.Text()
		counter++ /* line number */

		if strings.Contains(content, r.Oldtext) {
			r.Lock()
			r.Matches = append(r.Matches, Match{
				Filename:   filename,
				LineText:   content,
				LineNumber: counter,
				GrepFormat: fmt.Sprintf("%s:%d", filename, counter),
			})
			if !inArray(r.Uniques, filename) {
				r.Uniques = append(r.Uniques, filename)
			}
			r.Unlock()
		}
	}
}
