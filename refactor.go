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
