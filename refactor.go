package main

import (
	"bufio"
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

	// need at least two arguments, 1) the old text to search, and 2) the new
	// text to replace the old one.
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(1)
	}

	oldText := flag.Arg(0)
	newText := flag.Arg(1)

	if oldText == newText {
		fmt.Println("noop (A == B)")
		os.Exit(1)
	}

	// Assume the remaining program arguments are a list of files to search and
	// replace. Ignore the first two indeces in the array as they are the OLD
	// and NEW text to search and replace, respectively.
	files := flag.Args()[2:]

	// If the total number of program arguments provided by the user is two (2)
	// then the list of files will be empty, in which case we will need to run
	// a recursive file search.
	if flag.NArg() == 2 {
		files = findFilesRecursively()
	}

	var wg sync.WaitGroup
	sem := make(chan bool, 50)
	result := make(chan SearchResult)

	wg.Add(len(files))

	for _, filename := range files {
		go searchThisFile(sem, &wg, result, filename, oldText)
	}

	go func() {
		wg.Wait()
		close(result)
	}()

	for res := range result {
		if len(res.Findings) == 0 {
			continue
		}
		// wg.Add(1)
		// go modifyThisFile(sem, &wg, res)
		for _, item := range res.Findings {
			fmt.Printf(
				"\x1b[0;35m%s\x1b[0m:\x1b[0;32m%d\x1b[0m:%s\n",
				res.Filename,
				item.LineNumber,
				strings.Replace(
					item.OriginalText,
					oldText,
					"\x1b[1;31m"+oldText+"\x1b[0m",
					item.Occurrences,
				),
			)
		}
	}

	// wg.Wait()

	// searchAndReplace(
	// 	files,
	// flag.Arg(0),
	// flag.Arg(1),
	// )

	// if len(r.Matches) == 0 {
	// 	return errors.New("Nothing to refactor")
	// }

	// r.PrintMatches()

	// r.ReplaceMatches()
}

type SearchResult struct {
	Filename string
	Findings []Finding
}

type Finding struct {
	LineNumber   int
	Occurrences  int
	OriginalText string
}

// NewRefactor creates an instance of the Refactor interface.
func NewRefactor(oldtext string, newtext string, filelist []string) *Refactor {
	return &Refactor{
		Oldtext:  oldtext,
		Newtext:  newtext,
		Filelist: filelist,
	}
}

func findFilesRecursively() []string {
	filelist := []string{}
	if err := filepath.Walk(".", func(s string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		filelist = append(filelist, s)
		return nil
	}); err != nil {
		fmt.Println("filepath.Walk", err)
	}
	return filelist
}

// searchThisFile reads the content of a file and finds the query.
func searchThisFile(sem chan bool, wg *sync.WaitGroup, result chan SearchResult, filename string, query string) {
	sem <- true
	defer func() { <-sem }()
	defer func() { wg.Done() }()

	fi, err := os.Lstat(filename)

	if err != nil {
		fmt.Println("os.lstat:", filename, err)
		return
	}

	// skip files acting as symbolic links.
	if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		return
	}

	file, err := os.Open(filename)

	if err != nil {
		fmt.Println("os.Open", filename, err)
		return
	}

	defer func() {
		if err := file.Close(); err != nil {
			fmt.Println("file.Close", err)
		}
	}()

	var row int
	var line string
	var findings []Finding

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		row++ /* line number */
		line = scanner.Text()

		if n := strings.Count(line, query); n > 0 {
			findings = append(findings, Finding{
				LineNumber:   row,
				Occurrences:  n,
				OriginalText: line,
			})
		}
	}

	result <- SearchResult{Filename: filename, Findings: findings}
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
