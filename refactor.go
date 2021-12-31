package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
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

var flagCommitChanges bool

func main() {
	flag.BoolVar(&flagCommitChanges, "x", false, "Execute the replacement operation (default is preview-only)")

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

		wg.Add(1)

		go modifyThisFile(sem, &wg, res, oldText, newText)
	}

	wg.Wait()
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
	defer wg.Done()
	defer func() { <-sem }()

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

// modifyThisFile changes the content of the specified file.
func modifyThisFile(sem chan bool, wg *sync.WaitGroup, res SearchResult, oldText string, newText string) {
	sem <- true
	defer wg.Done()
	defer func() { <-sem }()

	// preview changes and exit.
	if !flagCommitChanges {
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

		return
	}

	content, err := ioutil.ReadFile(res.Filename)

	if err != nil {
		fmt.Println("ioutil.ReadFile", res.Filename, err)
		return
	}

	var totalOccurrences int

	for _, item := range res.Findings {
		fmt.Printf(
			"\x1b[0;35m%s\x1b[0m:\x1b[0;32m%d\x1b[0m:%s\n",
			res.Filename,
			item.LineNumber,
			strings.Replace(
				item.OriginalText,
				oldText,
				"\x1b[0;9m"+oldText+"\x1b[0m\x1b[1;34m"+newText+"\x1b[0m",
				item.Occurrences,
			),
		)
		totalOccurrences += item.Occurrences
	}

	content = bytes.Replace(content, []byte(oldText), []byte(newText), totalOccurrences)

	if err := ioutil.WriteFile(res.Filename, content, 0644); err != nil {
		fmt.Println("ioutil.WriteFile", res.Filename, err)
	}
}
