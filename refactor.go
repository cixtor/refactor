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
