//+build ignore

// This one generates types to work with prefix trees.
// Four types are currently generated:
// tree32
// tree64
// tree128
//
// Code in tree.go for tree160 is used as "standard".
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/importer"
	"io/ioutil"
	"os"
	"strings"
)

// Copyright (c) 2016 Alex Sergeyev. All rights reserved. See LICENSE file for terms of use.

var packageHdr = `// *** AUTOGENERATED BY "go generate" ***

package iptrie

import (
        "fmt"
        "unsafe"
)

`

var flagOut = flag.String("o", "tree_auto.go", "Where to write result")
var genMAXBITS = []string{"32", "64", "128"}

func main() {
	flag.Parse()

	_, err := importer.Default().Import("github.com/asergeyev/iptrie")
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not import:", err)
		os.Exit(1)
	}

	f, err := os.Open("tree160.go")
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not read:", err)
		os.Exit(1)
	}

	content, err := ioutil.ReadAll(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err read:", err)
		os.Exit(1)
	}

	content, err = format.Source(content)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err fmt:", err)
		os.Exit(1)
	}

	dst := bytes.NewBuffer(nil)
	dst.WriteString(packageHdr)

	pos := bytes.Index(content, []byte("//go:generate"))
	content = content[pos:]
	pos = bytes.IndexByte(content, '\n')
	content = content[pos:]

	for _, maxbits := range genMAXBITS {
		src := bytes.NewBuffer(content)
		for {
			definition, err := src.ReadString('\n')
			if err != nil {
				break
			}
			if strings.HasPrefix(definition, "package ") || strings.HasPrefix(definition, "const ") {
				continue
			}
			definition = strings.Replace(definition, "160", maxbits, -1)
			definition = strings.Replace(definition, "MAXBITS", maxbits, -1)
			definition = strings.Replace(definition, "[5]uint32", "["+maxbits+"/32]uint32", -1)
			fmt.Fprint(dst, definition)
		}
	}

	res, err := format.Source(dst.Bytes())
	if err != nil {
		fmt.Fprintln(os.Stderr, "err final fmt:", err)
		os.Exit(1)
	}

	ioutil.WriteFile(*flagOut, res, 0640)

}
