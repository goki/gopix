// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"sync"

	"github.com/goki/gi/gi"
	"github.com/goki/ki/ints"
)

// PProg is a parallel progress monitor
type PProg struct {
	Max int
	Inc int
	Cur int
	Mu  sync.Mutex
	Bar *gi.ScrollBar
}

func ProgInc(max int) int {
	switch {
	case max > 50000:
		return 1000
	case max > 5000:
		return 100
	}
	return 10
}

func (pp *PProg) Start(max int) {
	pp.Max = max - 1
	pp.Max = ints.MaxInt(1, pp.Max)
	pp.Inc = ProgInc(max)
	pp.Cur = 0
	pp.UpdtBar()
}

func (pp *PProg) UpdtBar() {
	updt := pp.Bar.UpdateStart()
	pp.Bar.SetThumbValue(float32(pp.Cur) / float32(pp.Max))
	pp.Bar.UpdateEnd(updt)
}

// Step is called by worker threads to update the current count
func (pp *PProg) Step() {
	pp.Mu.Lock()
	pp.Cur++
	if pp.Cur%pp.Inc == 0 {
		pp.UpdtBar()
	}
	pp.Mu.Unlock()
}
