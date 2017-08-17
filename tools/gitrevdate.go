// Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	out, err := exec.Command("git", "describe", "--always", "--dirty").Output()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	rev := string(out)
	if strings.HasSuffix(strings.TrimSpace(rev), "-dirty") {
		fmt.Println(time.Now().Format(time.RFC3339))
		return
	}

	out, err = exec.Command("git", "show", "--no-patch", "--date=iso-strict").Output()
	if err != nil {
		log.Println(err)
		os.Exit(2)
	}
	re := regexp.MustCompile(`(?m)^Date:\s+(.+)$`)
	m := re.FindSubmatch(out)[1]
	fmt.Println(string(m))

}
