package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Refactor defines the interface to process the files.
type Refactor struct {
	sync.Mutex
	Matches  []Match
	Uniques  []string
	Oldtext  string
	Newtext  string
	Filelist []string
}

// Match defines the structure of one single result.
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

// NewRefactor creates an instance of the Refactor interface.
func NewRefactor(oldtext string, newtext string, filelist []string) *Refactor {
	return &Refactor{
		Oldtext:  oldtext,
		Newtext:  newtext,
		Filelist: filelist,
	}
}

// Execute runs the file scanner and prints the results.
func (r *Refactor) Execute() error {
	if r.Oldtext == r.Newtext {
		return errors.New("Old and new text are the same")
	}

	if len(r.Filelist) < 2 {
		flag.Usage()
		os.Exit(1)
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

// FindFiles walks through the directory tree and returns the files.
func (r *Refactor) FindFiles() []string {
	filelist := []string{".", ".."}
	if err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if path[0] == '.' {
			return nil
		}
		filelist = append(filelist, path)
		return nil
	}); err != nil {
		log.Println("file.walk;", err)
	}
	return filelist
}

// GrepDirectory inspects the content of every file in the directory tree.
func (r *Refactor) GrepDirectory() {
	var wg sync.WaitGroup
	sema := make(chan int, 50)
	wg.Add(len(r.Filelist))
	for _, filename := range r.Filelist {
		go r.InspectFile(&wg, sema, filename)
	}
	wg.Wait()
}

// InspectFile reads the content of a file and finds the query.
func (r *Refactor) InspectFile(wg *sync.WaitGroup, sema chan int, filename string) {
	sema <- 1

	fi, err := os.Lstat(filename)

	if err != nil {
		fmt.Println("os.lstat:", filename, err)
		return
	}

	/* skip symlink files; they cannot be opened */
	if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		<-sema
		wg.Done()
		return
	}

	file, err := os.Open(filename)

	if err != nil {
		fmt.Println("os.open:", filename, err)
		return
	}

	defer func() {
		<-sema
		wg.Done()
		if err := file.Close(); err != nil {
			log.Println("file.close;", err)
		}
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

// PrintMatches sends the results to the standard output.
func (r *Refactor) PrintMatches() {
	var longest int
	var padding string
	var colored string

	for _, match := range r.Matches {
		if len(match.GrepFormat) > longest {
			longest = len(match.GrepFormat)
		}
	}

	fmt.Println("@ Refactoring Matches")

	for _, match := range r.Matches {
		padding = strings.Repeat("\x20", longest-len(match.GrepFormat))

		colored = strings.Replace(match.LineText,
			r.Oldtext, /* text that will be replaced */
			"\x1b[0;34m"+r.Oldtext+"\x1b[0m",
			-1)

		fmt.Printf("  %s:\x1b[0;31m%d\x1b[0m%s | %s\n",
			match.Filename,
			match.LineNumber,
			padding,
			colored)
	}
}

// ReplaceMatches rewrites the content of the files.
func (r *Refactor) ReplaceMatches() {
	var answer string

	fmt.Printf("@ Found %d occurrences; continue? [y/n] ", len(r.Matches))

	if _, err := fmt.Scanf("%s", &answer); err != nil {
		/* read user input to continue operation */
		log.Println("fmt.scanf;", err)
		return
	}

	if answer != "y" {
		fmt.Println("@ Canceling Refactoring")
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(r.Uniques))
	for _, filename := range r.Uniques {
		go r.ModifyFileContent(&wg, filename)
	}
	wg.Wait()

	fmt.Println("@ Finished")
}

// ModifyFileContent changes the content of the specified file.
func (r *Refactor) ModifyFileContent(wg *sync.WaitGroup, filename string) {
	defer wg.Done()

	read, err := ioutil.ReadFile(filename)

	if err != nil {
		fmt.Println("  Error refactoring", filename+";", err)
		return
	}

	content := strings.Replace(string(read), r.Oldtext, r.Newtext, -1)

	if err := ioutil.WriteFile(filename, []byte(content), 0); err != nil {
		fmt.Println("  Error writing", filename+";", err)
	}

	fmt.Printf("  \x1b[0;32mOK\x1b[0m %s\n", filename)
}

func inArray(haystack []string, needle string) bool {
	for _, text := range haystack {
		if text == needle {
			return true
		}
	}
	return false
}
