// Copyright (c) 2020, The gide / Goki Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/oswin"
)

func main() {
	oswin.TheApp.SetName("gopix")
	oswin.TheApp.SetAbout(`<code>GoPix</code> Is a Go picture management system within the <b>Goki</b> tree framework.  See <a href="https://goki.dev/gopix">GoPix on GitHub</a>`)

	// oswin.TheApp.SetQuitCleanFunc(func() {
	// 	fmt.Printf("Doing final Quit cleanup here..\n")
	// })

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(usr.HomeDir, "Pix")

	// process command args
	if len(os.Args) > 1 {
		flag.StringVar(&path, "path", "", "path to open -- can be to a directory or a filename within the directory ")
		// todo: other args?
		flag.Parse()
	}

	pv, _ := GoPixViewWindow(path)
	_ = pv

	gi.WinWait.Wait()
}
