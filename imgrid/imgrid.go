// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imgrid

import (
	"fmt"
	"image"
	"sort"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/dnd"
	"github.com/goki/gi/oswin/key"
	"github.com/goki/gi/oswin/mimedata"
	"github.com/goki/gi/oswin/mouse"
	"github.com/goki/gi/units"
	"github.com/goki/ki/ints"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/ki/sliceclone"
	"github.com/goki/mat32"
	"github.com/goki/pi/filecat"
)

// ImgGrid shows a list of images in a grid.
// The outer layout contains the inner grid and a scrollbar
type ImgGrid struct {
	gi.Frame
	ImageMax     float32          `desc:"maximum size for images -- geom set to square of this size"`
	Size         image.Point      `desc:"number of columns and rows to display"`
	Images       []string         `desc:"list of image files to display"`
	SelectedIdx  int              `desc:"last selected item"`
	SelectMode   bool             `copy:"-" desc:"editing-mode select rows mode"`
	SelectedIdxs map[int]struct{} `copy:"-" desc:"list of currently-selected file indexes"`
	DraggedIdxs  []int            `copy:"-" desc:"list of currently-dragged indexes"`
	ImageSig     ki.Signal        `copy:"-" json:"-" xml:"-" desc:"signal for image events -- selection events occur via WidgetSig"`
	CurIdx       int              `copy:"-" json:"-" xml:"-" desc:"current copy / paste idx"`
	InfoFunc     func(idx int)    `desc:"function for displaying file at given index"`
}

var KiT_ImgGrid = kit.Types.AddType(&ImgGrid{}, ImgGridProps)

// AddNewImgGrid adds a new imggrid to given parent node, with given name.
func AddNewImgGrid(parent ki.Ki, name string) *ImgGrid {
	return parent.AddNewChild(KiT_ImgGrid, name).(*ImgGrid)
}

// SetImages sets the current image files to view (makes a copy of slice),
// and does a config rebuild
func (ig *ImgGrid) SetImages(files []string) {
	ig.Images = sliceclone.String(files)
	ig.Config()
}

func (ig *ImgGrid) NumImages() int {
	return len(ig.Images)
}

// Config configures the grid
func (ig *ImgGrid) Config() {
	updt := ig.UpdateStart()
	defer ig.UpdateEnd(updt)
	ig.SetFullReRender()
	ig.SetCanFocus()

	if !ig.HasChildren() {
		ig.Lay = gi.LayoutHoriz
		gi.AddNewLayout(ig, "grid", gi.LayoutGrid)
		sbb := gi.AddNewScrollBar(ig, "sb")
		sbb.Defaults()
		sbb.SliderSig.Connect(ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.(*ImgGrid)
			if sig == int64(gi.SliderValueChanged) {
				igg.Update()
			}
		})
	}
	ig.ResetSelectedIdxs()
	ig.SelectedIdx = 0
	gr := ig.Grid()
	sb := ig.ScrollBar()
	if ig.Size.X == 0 {
		ig.Size.X = 4
		ig.Size.Y = 4
	}
	if ig.ImageMax == 0 {
		ig.ImageMax = 200
	}
	gr.SetProp("columns", ig.Size.X)
	gr.Lay = gi.LayoutGrid
	gr.SetStretchMax()
	gr.SetProp("spacing", gi.StdDialogVSpaceUnits)
	ng := ig.Size.X * ig.Size.Y
	if ng != gr.NumChildren() {
		gr.SetNChildren(ng, gi.KiT_Bitmap, "b_")
	}
	sb.Min = 0
	sb.Tracking = true
	sb.Dim = mat32.Y
	sb.SetFixedWidth(units.NewPx(gi.ScrollBarWidthDefault))
	sb.SetStretchMaxHeight()
	ig.SetScrollMax()
	ig.Update()
}

func (ig *ImgGrid) SetScrollMax() int {
	sb := ig.ScrollBar()
	nf := ig.NumImages()
	nr := int(mat32.Ceil(float32(nf)/float32(ig.Size.X))) + 1
	nr = ints.MaxInt(nr, ig.Size.Y)
	sb.Max = float32(nr)
	sb.ThumbVal = float32(ig.Size.Y)
	return nr
}

// Grid returns the actual grid layout
func (ig *ImgGrid) Grid() *gi.Layout {
	return ig.Child(0).(*gi.Layout)
}

// ScrollBar returns the scrollbar
func (ig *ImgGrid) ScrollBar() *gi.ScrollBar {
	return ig.Child(1).(*gi.ScrollBar)
}

// BitmapAtIdx returns the gi.Bitmap at given index
func (ig *ImgGrid) BitmapAtIdx(idx int) *gi.Bitmap {
	ni := ig.Size.X * ig.Size.Y
	if idx < 0 || idx >= ni {
		return nil
	}
	gr := ig.Grid()
	return gr.Child(idx).(*gi.Bitmap)
}

// ImageDeleteAt deletes image at given index
func (ig *ImgGrid) ImageDeleteAt(idx int) {
	// img := ig.Images[idx]
	ig.Images = append(ig.Images[:idx], ig.Images[idx+1:]...)
	ig.ImageSig.Emit(ig.This(), int64(ImgGridDeleted), idx)
}

// ImageInsertAt inserts image(s) at given index
func (ig *ImgGrid) ImageInsertAt(idx int, files []string) {
	ni := len(files)
	nt := append(ig.Images, files...) // first append to end
	copy(nt[idx+ni:], nt[idx:])       // move stuff to end
	copy(nt[idx:], files)             // copy into position
	ig.Images = nt
	ig.ImageSig.Emit(ig.This(), int64(ImgGridInserted), idx)
}

// ImgGridSignals are signals that sliceview can send, mostly for editing
// mode.  Selection events are sent on WidgetSig WidgetSelected signals in
// both modes.
type ImgGridSignals int

const (
	// ImgGridDoubleClicked emitted during inactive mode when item
	// double-clicked -- can be used for accepting dialog.
	ImgGridDoubleClicked ImgGridSignals = iota

	// ImgGridInserted emitted when a new item is inserted -- data is index of new item
	ImgGridInserted

	// ImgGridDeleted emitted when an item is deleted -- data is index of item deleted
	ImgGridDeleted

	ImgGridSignalsN
)

//go:generate stringer -type=ImgGridSignals

// LayoutGrid updates the grid size based on allocated size
// if returns true, then needs a new iteration
func (ig *ImgGrid) LayoutGrid(iter int) bool {
	if iter > 0 {
		return false
	}
	sb := ig.ScrollBar()
	gr := ig.Grid()

	alc := ig.LayState.Alloc.Size.ToPoint()
	alc.X -= int(sb.Sty.Layout.Width.Dots)
	gsz := alc.Div(int(ig.ImageMax))
	gsz.X = ints.MaxInt(2, gsz.X)
	gsz.Y = ints.MaxInt(2, gsz.Y)
	if ig.Size == gsz {
		return false
	}
	ig.Size = gsz
	gr.SetProp("columns", ig.Size.X)
	ig.SetScrollMax()
	ig.Update()
	return true
}

func (ig *ImgGrid) Layout2D(parBBox image.Rectangle, iter int) bool {
	redo := ig.LayoutGrid(iter)
	ig.Frame.Layout2D(parBBox, iter)
	return redo
}

// Update updates the display for current scrollbar position, rendering the images
func (ig *ImgGrid) Update() {
	updt := ig.UpdateStart()
	defer ig.UpdateEnd(updt)

	gr := ig.Grid()
	nf := ig.NumImages()
	ig.SetScrollMax()
	ng := ig.Size.X * ig.Size.Y
	if ng != gr.NumChildren() {
		gr.SetNChildren(ng, gi.KiT_Bitmap, "b_")
	}

	bimg := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	si := ig.StartIdx()
	idx := si
	bi := 0
	for y := 0; y < ig.Size.Y; y++ {
		for x := 0; x < ig.Size.X; x++ {
			bm := gr.Child(bi).(*gi.Bitmap)
			if idx < nf {
				f := ig.Images[idx]
				if f != "" {
					bm.OpenImage(gi.FileName(f), 0, 0)
				} else {
					bm.SetImage(bimg, 0, 0)
				}
			} else {
				bm.SetImage(bimg, 0, 0)
			}
			bm.SetProp("width", units.NewValue(float32(ig.ImageMax), units.Dot))
			bm.SetProp("height", units.NewValue(float32(ig.ImageMax), units.Dot))
			bi++
			idx++
		}
	}
}

func (ig *ImgGrid) RenderSelected() {
	gr := ig.Grid()

	st := &ig.Sty
	rs := &ig.Viewport.Render
	pc := &rs.Paint

	pc.StrokeStyle.SetColor(gi.Prefs.Colors.Select)
	pc.StrokeStyle.Width = st.Border.Width
	pc.FillStyle.SetColor(nil)
	wd := pc.StrokeStyle.Width.Dots

	si := ig.StartIdx()
	idx := si
	bi := 0
	for y := 0; y < ig.Size.Y; y++ {
		for x := 0; x < ig.Size.X; x++ {
			bm := gr.Child(bi).(*gi.Bitmap)
			if _, sel := ig.SelectedIdxs[idx]; sel {
				pos := bm.LayState.Alloc.Pos.SubScalar(wd)
				sz := bm.LayState.Alloc.Size.AddScalar(2.0 * wd)
				pc.DrawRectangle(rs, pos.X, pos.Y, sz.X, sz.Y)
			}
			bi++
			idx++
		}
	}
}

func (ig *ImgGrid) Render2D() {
	if ig.FullReRenderIfNeeded() {
		return
	}
	if ig.PushBounds() {
		ig.FrameStdRender()
		ig.This().(gi.Node2D).ConnectEvents2D()
		if ig.ScrollsOff {
			ig.ManageOverflow()
		}
		ig.RenderScrolls()
		ig.Render2DChildren()
		ig.RenderSelected()
		ig.PopBounds()
	} else {
		ig.SetScrollsOff()
		ig.DisconnectAllEvents(gi.AllPris) // uses both Low and Hi
	}
}

func (ig *ImgGrid) ConnectEvents2D() {
	ig.ImgGridEvents()
}

func (ig *ImgGrid) ImgGridEvents() {
	gr := ig.Grid()

	// LowPri to allow other focal widgets to capture
	ig.ConnectEvent(oswin.MouseScrollEvent, gi.LowPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		me := d.(*mouse.ScrollEvent)
		igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		me.SetProcessed()
		sbb := igg.ScrollBar()
		cur := float32(sbb.Pos)
		sbb.SliderMove(cur, cur+float32(me.NonZeroDelta(false))) // preferY
	})
	ig.ConnectEvent(oswin.MouseEvent, gi.LowRawPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		me := d.(*mouse.Event)
		igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		switch {
		case me.Button == mouse.Left && me.Action == mouse.DoubleClick:
			si := igg.SelectedIdx
			igg.UnselectAllIdxs()
			igg.SelectIdx(si)
			igg.ImageSig.Emit(igg.This(), int64(ImgGridDoubleClicked), si)
			me.SetProcessed()
		case me.Button == mouse.Left:
			idx, ok := igg.IdxFromPos(me.Pos())
			if !ok {
				return
			}
			me.SetProcessed()
			igg.GrabFocus()
			igg.SelectIdxAction(idx+ig.StartIdx(), me.SelectMode())
		case me.Button == mouse.Right && me.Action == mouse.Release:
			igg.ItemCtxtMenu(igg.SelectedIdx)
			me.SetProcessed()
		}
	})
	ig.ConnectEvent(oswin.KeyChordEvent, gi.HiPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		kt := d.(*key.ChordEvent)
		igg.KeyInputActive(kt)
	})
	ig.ConnectEvent(oswin.DNDEvent, gi.RegPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		de := d.(*dnd.Event)
		igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		switch de.Action {
		case dnd.Start:
			igg.DragNDropStart()
		case dnd.DropOnTarget:
			igg.DragNDropTarget(de)
		case dnd.DropFmSource:
			igg.DragNDropSource(de)
		}
	})
	gr.ConnectEvent(oswin.DNDFocusEvent, gi.RegPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		de := d.(*dnd.FocusEvent)
		sgg := recv.Embed(gi.KiT_Layout).(*gi.Layout)
		switch de.Action {
		case dnd.Enter:
			sgg.Viewport.Win.DNDSetCursor(de.Mod)
		case dnd.Exit:
			sgg.Viewport.Win.DNDNotCursor()
		case dnd.Hover:
			// nothing here?
		}
	})
}

/////////////////////////////////////////////////////////////////////////////
//    Moving

// MoveDown moves the selection down to next row, using given select mode
// (from keyboard modifiers) -- returns newly selected row or -1 if failed
func (ig *ImgGrid) MoveDown(selMode mouse.SelectModes) int {
	nf := ig.NumImages()
	if ig.SelectedIdx >= nf-1 {
		ig.SelectedIdx = nf - 1
		return -1
	}
	ig.SelectedIdx++
	ig.SelectIdxAction(ig.SelectedIdx, selMode)
	return ig.SelectedIdx
}

// MoveDownAction moves the selection down to next row, using given select
// mode (from keyboard modifiers) -- and emits select event for newly selected
// row
func (ig *ImgGrid) MoveDownAction(selMode mouse.SelectModes) int {
	nidx := ig.MoveDown(selMode)
	if nidx >= 0 {
		ig.ScrollToIdx(nidx)
		ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), nidx)
	}
	return nidx
}

// MoveUp moves the selection up to previous idx, using given select mode
// (from keyboard modifiers) -- returns newly selected idx or -1 if failed
func (ig *ImgGrid) MoveUp(selMode mouse.SelectModes) int {
	if ig.SelectedIdx <= 0 {
		ig.SelectedIdx = 0
		return -1
	}
	ig.SelectedIdx--
	ig.SelectIdxAction(ig.SelectedIdx, selMode)
	return ig.SelectedIdx
}

// MoveUpAction moves the selection up to previous idx, using given select
// mode (from keyboard modifiers) -- and emits select event for newly selected idx
func (ig *ImgGrid) MoveUpAction(selMode mouse.SelectModes) int {
	nidx := ig.MoveUp(selMode)
	if nidx >= 0 {
		ig.ScrollToIdx(nidx)
		ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), nidx)
	}
	return nidx
}

// MovePageDown moves the selection down to next page, using given select mode
// (from keyboard modifiers) -- returns newly selected idx or -1 if failed
func (ig *ImgGrid) MovePageDown(selMode mouse.SelectModes) int {
	nf := ig.NumImages()
	if ig.SelectedIdx >= nf-1 {
		ig.SelectedIdx = nf - 1
		return -1
	}
	ig.SelectedIdx += ig.Size.X * ig.Size.Y
	ig.SelectedIdx = ints.MinInt(ig.SelectedIdx, nf-1)
	ig.SelectIdxAction(ig.SelectedIdx, selMode)
	return ig.SelectedIdx
}

// MovePageDownAction moves the selection down to next page, using given select
// mode (from keyboard modifiers) -- and emits select event for newly selected idx
func (ig *ImgGrid) MovePageDownAction(selMode mouse.SelectModes) int {
	nidx := ig.MovePageDown(selMode)
	if nidx >= 0 {
		ig.ScrollToIdx(nidx)
		ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), nidx)
	}
	return nidx
}

// MovePageUp moves the selection up to previous page, using given select mode
// (from keyboard modifiers) -- returns newly selected idx or -1 if failed
func (ig *ImgGrid) MovePageUp(selMode mouse.SelectModes) int {
	if ig.SelectedIdx <= 0 {
		ig.SelectedIdx = 0
		return -1
	}
	ig.SelectedIdx -= ig.Size.X * ig.Size.Y
	ig.SelectedIdx = ints.MaxInt(0, ig.SelectedIdx)
	ig.SelectIdxAction(ig.SelectedIdx, selMode)
	return ig.SelectedIdx
}

// MovePageUpAction moves the selection up to previous page, using given select
// mode (from keyboard modifiers) -- and emits select event for newly selected idx
func (ig *ImgGrid) MovePageUpAction(selMode mouse.SelectModes) int {
	nidx := ig.MovePageUp(selMode)
	if nidx >= 0 {
		ig.ScrollToIdx(nidx)
		ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), nidx)
	}
	return nidx
}

//////////////////////////////////////////////////////////////////////////////
//    Selection: user operates on the index labels

// StartIdx returns the index of first image visible
func (ig *ImgGrid) StartIdx() int {
	sb := ig.ScrollBar()
	si := int(sb.Value) * ig.Size.X
	return si
}

// IsIdxVisible returns true if image index is currently visible
func (ig *ImgGrid) IsIdxVisible(idx int) bool {
	si := ig.StartIdx()
	sidx := si
	eidx := sidx + ig.Size.X*ig.Size.Y
	if idx < sidx || idx >= eidx {
		return false
	}
	return true
}

// IdxPos returns center of window position of index label for idx (ContextMenuPos)
func (ig *ImgGrid) IdxPos(idx int) image.Point {
	si := ig.StartIdx()
	bi := idx - si
	if bi < 0 {
		bi = 0
	}
	ni := ig.Size.X * ig.Size.Y
	if bi > ni-1 {
		bi = ni - 1
	}
	var pos image.Point
	bm := ig.BitmapAtIdx(bi)
	if bm != nil {
		pos = bm.ContextMenuPos()
	}
	return pos
}

// IdxFromPos returns the widget grid idx that contains given position, false if not found
// add StartIdx to get actual index
func (ig *ImgGrid) IdxFromPos(pos image.Point) (int, bool) {
	gr := ig.Grid()
	st := gr.WinBBox.Min
	rp := pos.Sub(st)
	if rp.X < 0 || rp.Y < 0 {
		return 0, false
	}
	sp := 2 * gr.Spacing.Dots
	x := rp.X / int(ig.ImageMax+sp)
	x = ints.MinInt(x, ig.Size.X)
	y := rp.Y / int(ig.ImageMax+sp)
	y = ints.MinInt(y, ig.Size.Y)
	idx := y*ig.Size.X + x
	return idx, true
}

// ScrollToIdx ensures that given slice idx is visible by scrolling display as needed
func (ig *ImgGrid) ScrollToIdx(idx int) bool {
	sb := ig.ScrollBar()
	si := ig.StartIdx()
	ir := idx / ig.Size.X
	np := ig.Size.X * ig.Size.Y
	if idx < si {
		sb.SetValueAction(float32(ir))
		return true
	} else if idx >= si+np {
		ni := ir - ig.Size.Y - 1
		ni = ints.MaxInt(0, ni)
		sb.SetValueAction(float32(ni))
		return true
	}
	return false
}

// SelectIdxWidgets sets the selection state of given slice index
// returns false if index is not visible
func (ig *ImgGrid) SelectIdxWidgets(idx int, sel bool) bool {
	if !ig.IsIdxVisible(idx) {
		return false
	}
	ig.UpdateSig()
	return true
}

// IdxGrabFocus grabs the focus for the first focusable widget in given idx
func (ig *ImgGrid) IdxGrabFocus(idx int) {
	ig.ScrollToIdx(idx)

}

// UpdateSelectIdx updates the selection for the given index
func (ig *ImgGrid) UpdateSelectIdx(idx int, sel bool) {
	selMode := mouse.SelectOne
	em := ig.EventMgr2D()
	if em != nil {
		selMode = em.LastSelMode
	}
	ig.SelectIdxAction(idx, selMode)
}

// IdxIsSelected returns the selected status of given slice index
func (ig *ImgGrid) IdxIsSelected(idx int) bool {
	if _, ok := ig.SelectedIdxs[idx]; ok {
		return true
	}
	return false
}

func (ig *ImgGrid) ResetSelectedIdxs() {
	ig.SelectedIdxs = make(map[int]struct{})
}

// SelectedIdxsList returns list of selected indexes, sorted either ascending or descending
func (ig *ImgGrid) SelectedIdxsList(descendingSort bool) []int {
	rws := make([]int, len(ig.SelectedIdxs))
	i := 0
	for r, _ := range ig.SelectedIdxs {
		rws[i] = r
		i++
	}
	if descendingSort {
		sort.Slice(rws, func(i, j int) bool {
			return rws[i] > rws[j]
		})
	} else {
		sort.Slice(rws, func(i, j int) bool {
			return rws[i] < rws[j]
		})
	}
	return rws
}

// SelectIdx selects given idx (if not already selected) -- updates select
// status of index label
func (ig *ImgGrid) SelectIdx(idx int) {
	ig.SelectedIdxs[idx] = struct{}{}
	ig.SelectIdxWidgets(idx, true)
}

// UnselectIdx unselects given idx (if selected)
func (ig *ImgGrid) UnselectIdx(idx int) {
	if ig.IdxIsSelected(idx) {
		delete(ig.SelectedIdxs, idx)
	}
	ig.SelectIdxWidgets(idx, false)
}

// UnselectAllIdxs unselects all selected idxs
func (ig *ImgGrid) UnselectAllIdxs() {
	updt := ig.UpdateStart()
	ig.ResetSelectedIdxs()
	ig.UpdateEnd(updt)
}

// SelectAllIdxs selects all idxs
func (ig *ImgGrid) SelectAllIdxs() {
	nf := ig.NumImages()
	updt := ig.UpdateStart()
	ig.UnselectAllIdxs()
	ig.SelectedIdxs = make(map[int]struct{}, nf)
	for idx := 0; idx < nf; idx++ {
		ig.SelectedIdxs[idx] = struct{}{}
	}
	ig.UpdateEnd(updt)
}

// SelectIdxAction is called when a select action has been received (e.g., a
// mouse click) -- translates into selection updates -- gets selection mode
// from mouse event (ExtendContinuous, ExtendOne)
func (ig *ImgGrid) SelectIdxAction(idx int, mode mouse.SelectModes) {
	if mode == mouse.NoSelect {
		return
	}
	nf := ig.NumImages()
	idx = ints.MinInt(idx, nf-1)
	if idx < 0 {
		idx = 0
	}
	// row := idx - sv.StartIdx // note: could be out of bounds
	switch mode {
	case mouse.SelectOne:
		if ig.IdxIsSelected(idx) {
			if len(ig.SelectedIdxs) > 1 {
				ig.UnselectAllIdxs()
			}
			ig.SelectedIdx = idx
			ig.SelectIdx(idx)
			ig.IdxGrabFocus(idx)
		} else {
			ig.UnselectAllIdxs()
			ig.SelectedIdx = idx
			ig.SelectIdx(idx)
			ig.IdxGrabFocus(idx)
		}
		ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), ig.SelectedIdx)
	case mouse.ExtendContinuous:
		if len(ig.SelectedIdxs) == 0 {
			ig.SelectedIdx = idx
			ig.SelectIdx(idx)
			ig.IdxGrabFocus(idx)
			ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), ig.SelectedIdx)
		} else {
			minIdx := -1
			maxIdx := 0
			for r, _ := range ig.SelectedIdxs {
				if minIdx < 0 {
					minIdx = r
				} else {
					minIdx = ints.MinInt(minIdx, r)
				}
				maxIdx = ints.MaxInt(maxIdx, r)
			}
			cidx := idx
			ig.SelectedIdx = idx
			ig.SelectIdx(idx)
			if idx < minIdx {
				for cidx < minIdx {
					r := ig.MoveDown(mouse.SelectQuiet) // just select
					cidx = r
				}
			} else if idx > maxIdx {
				for cidx > maxIdx {
					r := ig.MoveUp(mouse.SelectQuiet) // just select
					cidx = r
				}
			}
			ig.IdxGrabFocus(idx)
			ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), ig.SelectedIdx)
		}
	case mouse.ExtendOne:
		if ig.IdxIsSelected(idx) {
			ig.UnselectIdxAction(idx)
		} else {
			ig.SelectedIdx = idx
			ig.SelectIdx(idx)
			ig.IdxGrabFocus(idx)
			ig.WidgetSig.Emit(ig.This(), int64(gi.WidgetSelected), ig.SelectedIdx)
		}
	case mouse.Unselect:
		ig.SelectedIdx = idx
		ig.UnselectIdxAction(idx)
	case mouse.SelectQuiet:
		ig.SelectedIdx = idx
		ig.SelectIdx(idx)
	case mouse.UnselectQuiet:
		ig.SelectedIdx = idx
		ig.UnselectIdx(idx)
	}
	ig.Update()
}

// UnselectIdxAction unselects this idx (if selected) -- and emits a signal
func (ig *ImgGrid) UnselectIdxAction(idx int) {
	if ig.IdxIsSelected(idx) {
		ig.UnselectIdx(idx)
	}
}

//////////////////////////////////////////////////////////////////////////////
//    Copy / Cut / Paste

// MimeDataIdx adds mimedata for given idx: an application/json of the struct
func (ig *ImgGrid) MimeDataIdx(md *mimedata.Mimes, idx int) {
	fn := ig.Images[idx]
	*md = append(*md, &mimedata.Data{Type: filecat.TextPlain, Data: []byte(fn)})
}

// FromMimeData creates a slice of file names from mime data
func (ig *ImgGrid) FromMimeData(md mimedata.Mimes) []string {
	var sl []string
	for _, d := range md {
		if d.Type == filecat.TextPlain {
			fn := string(d.Data)
			sl = append(sl, fn)
		}
	}
	return sl
}

// CopySelToMime copies selected rows to mime data
func (ig *ImgGrid) CopySelToMime() mimedata.Mimes {
	nitms := len(ig.SelectedIdxs)
	if nitms == 0 {
		return nil
	}
	ixs := ig.SelectedIdxsList(false) // ascending
	md := make(mimedata.Mimes, 0, nitms)
	for _, i := range ixs {
		ig.MimeDataIdx(&md, i)
	}
	return md
}

// Copy copies selected rows to clip.Board, optionally resetting the selection
// satisfies gi.Clipper interface and can be overridden by subtypes
func (ig *ImgGrid) Copy(reset bool) {
	nitms := len(ig.SelectedIdxs)
	if nitms == 0 {
		return
	}
	md := ig.CopySelToMime()
	if md != nil {
		oswin.TheApp.ClipBoard(ig.Viewport.Win.OSWin).Write(md)
	}
	if reset {
		ig.UnselectAllIdxs()
	}
}

// CopyIdxs copies selected idxs to clip.Board, optionally resetting the selection
func (ig *ImgGrid) CopyIdxs(reset bool) {
	if cpr, ok := ig.This().(gi.Clipper); ok { // should always be true, but justin case..
		cpr.Copy(reset)
	} else {
		ig.Copy(reset)
	}
}

// DeleteIdxs deletes all selected indexes
func (ig *ImgGrid) DeleteIdxs() {
	if len(ig.SelectedIdxs) == 0 {
		return
	}
	updt := ig.UpdateStart()
	ixs := ig.SelectedIdxsList(true) // descending sort
	for _, i := range ixs {
		ig.ImageDeleteAt(i)
	}
	ig.UpdateEnd(updt)
}

// Cut copies selected indexes to clip.Board and deletes selected indexes
// satisfies gi.Clipper interface and can be overridden by subtypes
func (ig *ImgGrid) Cut() {
	if len(ig.SelectedIdxs) == 0 {
		return
	}
	ig.CopyIdxs(false)
	ixs := ig.SelectedIdxsList(true) // descending sort
	idx := ixs[0]
	ig.UnselectAllIdxs()
	for _, i := range ixs {
		ig.ImageDeleteAt(i)
	}
	ig.Update()
	ig.SelectIdxAction(idx, mouse.SelectOne)
}

// CutIdxs copies selected indexes to clip.Board and deletes selected indexes
func (ig *ImgGrid) CutIdxs() {
	if cpr, ok := ig.This().(gi.Clipper); ok { // should always be true, but justin case..
		cpr.Cut()
	} else {
		ig.Cut()
	}
}

// Paste pastes clipboard at CurIdx
// satisfies gi.Clipper interface and can be overridden by subtypes
func (ig *ImgGrid) Paste() {
	md := oswin.TheApp.ClipBoard(ig.Viewport.Win.OSWin).Read([]string{filecat.TextPlain})
	if md != nil {
		ig.PasteMenu(md, ig.CurIdx)
	}
}

// PasteIdx pastes clipboard at given idx
func (ig *ImgGrid) PasteIdx(idx int) {
	ig.CurIdx = idx
	if cpr, ok := ig.This().(gi.Clipper); ok { // should always be true, but justin case..
		cpr.Paste()
	} else {
		ig.Paste()
	}
}

// MakePasteMenu makes the menu of options for paste events
func (ig *ImgGrid) MakePasteMenu(m *gi.Menu, data interface{}, idx int) {
	if len(*m) > 0 {
		return
	}
	m.AddAction(gi.ActOpts{Label: "Assign To", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		svv.PasteAssign(data.(mimedata.Mimes), idx)
	})
	m.AddAction(gi.ActOpts{Label: "Insert Before", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		svv.PasteAtIdx(data.(mimedata.Mimes), idx)
	})
	m.AddAction(gi.ActOpts{Label: "Insert After", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		svv.PasteAtIdx(data.(mimedata.Mimes), idx+1)
	})
	m.AddAction(gi.ActOpts{Label: "Cancel", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
	})
}

// PasteMenu performs a paste from the clipboard using given data -- pops up
// a menu to determine what specifically to do
func (ig *ImgGrid) PasteMenu(md mimedata.Mimes, idx int) {
	ig.UnselectAllIdxs()
	var men gi.Menu
	ig.MakePasteMenu(&men, md, idx)
	pos := ig.IdxPos(idx)
	gi.PopupMenu(men, pos.X, pos.Y, ig.Viewport, "svPasteMenu")
}

// PasteAssign assigns mime data (only the first one!) to this idx
func (ig *ImgGrid) PasteAssign(md mimedata.Mimes, idx int) {
	sl := ig.FromMimeData(md)
	if len(sl) == 0 {
		return
	}
	updt := ig.UpdateStart()
	ig.SetFullReRender()
	ns := sl[0]
	ig.Images[idx] = ns
	ig.UpdateEnd(updt)
}

// PasteAtIdx inserts object(s) from mime data at (before) given slice index
func (ig *ImgGrid) PasteAtIdx(md mimedata.Mimes, idx int) {
	sl := ig.FromMimeData(md)
	if len(sl) == 0 {
		return
	}
	ig.ImageInsertAt(idx, sl)
	ig.Update()
	ig.SelectIdxAction(idx, mouse.SelectOne)
}

// Duplicate copies selected items and inserts them after current selection --
// return idx of start of duplicates if successful, else -1
func (ig *ImgGrid) Duplicate() int {
	nitms := len(ig.SelectedIdxs)
	if nitms == 0 {
		return -1
	}
	ixs := ig.SelectedIdxsList(true) // descending sort -- last first
	pasteAt := ixs[0]
	ig.CopyIdxs(true)
	md := oswin.TheApp.ClipBoard(ig.Viewport.Win.OSWin).Read([]string{filecat.TextPlain})
	ig.PasteAtIdx(md, pasteAt)
	return pasteAt
}

//////////////////////////////////////////////////////////////////////////////
//    Drag-n-Drop

// DragNDropStart starts a drag-n-drop
func (ig *ImgGrid) DragNDropStart() {
	nitms := len(ig.SelectedIdxs)
	if nitms == 0 {
		return
	}
	md := ig.CopySelToMime()
	ixs := ig.SelectedIdxsList(false) // ascending
	bm := ig.BitmapAtIdx(ixs[0])
	if bm != nil {
		sp := &gi.Sprite{}
		sp.Pixels = gi.ImageResizeMax(bm.Pixels, 32).(*image.RGBA)
		gi.ImageClearer(sp.Pixels, 50.0)
		ig.Viewport.Win.StartDragNDrop(ig.This(), md, sp)
	}
}

// DragNDropTarget handles a drag-n-drop drop
func (ig *ImgGrid) DragNDropTarget(de *dnd.Event) {
	de.Target = ig.This()
	if de.Mod == dnd.DropLink {
		de.Mod = dnd.DropCopy // link not supported -- revert to copy
	}
	idx, ok := ig.IdxFromPos(de.Where)
	if ok {
		de.SetProcessed()
		ig.CurIdx = ig.StartIdx() + idx
		if dpr, ok := ig.This().(gi.DragNDropper); ok {
			dpr.Drop(de.Data, de.Mod)
		} else {
			ig.Drop(de.Data, de.Mod)
		}
	}
}

// MakeDropMenu makes the menu of options for dropping on a target
func (ig *ImgGrid) MakeDropMenu(m *gi.Menu, data interface{}, mod dnd.DropMods, idx int) {
	if len(*m) > 0 {
		return
	}
	switch mod {
	case dnd.DropCopy:
		m.AddLabel("Copy (Shift=Move):")
	case dnd.DropMove:
		m.AddLabel("Move:")
	}
	if mod == dnd.DropCopy {
		m.AddAction(gi.ActOpts{Label: "Assign To", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			svv.DropAssign(data.(mimedata.Mimes), idx)
		})
	}
	m.AddAction(gi.ActOpts{Label: "Insert Before", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		svv.DropBefore(data.(mimedata.Mimes), mod, idx) // captures mod
	})
	m.AddAction(gi.ActOpts{Label: "Insert After", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		svv.DropAfter(data.(mimedata.Mimes), mod, idx) // captures mod
	})
	m.AddAction(gi.ActOpts{Label: "Cancel", Data: data}, ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv := recv.Embed(KiT_ImgGrid).(*ImgGrid)
		svv.DropCancel()
	})
}

// Drop pops up a menu to determine what specifically to do with dropped items
// this satisfies gi.DragNDropper interface, and can be overwritten in subtypes
func (ig *ImgGrid) Drop(md mimedata.Mimes, mod dnd.DropMods) {
	var men gi.Menu
	ig.MakeDropMenu(&men, md, mod, ig.CurIdx)
	pos := ig.IdxPos(ig.CurIdx)
	gi.PopupMenu(men, pos.X, pos.Y, ig.Viewport, "svDropMenu")
}

// DropAssign assigns mime data (only the first one!) to this node
func (ig *ImgGrid) DropAssign(md mimedata.Mimes, idx int) {
	ig.DraggedIdxs = nil
	ig.PasteAssign(md, idx)
	ig.DragNDropFinalize(dnd.DropCopy)
}

// DragNDropFinalize is called to finalize actions on the Source node prior to
// performing target actions -- mod must indicate actual action taken by the
// target, including ignore -- ends up calling DragNDropSource if us..
func (ig *ImgGrid) DragNDropFinalize(mod dnd.DropMods) {
	ig.UnselectAllIdxs()
	ig.Viewport.Win.FinalizeDragNDrop(mod)
}

// DragNDropSource is called after target accepts the drop -- we just remove
// elements that were moved
func (ig *ImgGrid) DragNDropSource(de *dnd.Event) {
	if de.Mod != dnd.DropMove || len(ig.DraggedIdxs) == 0 {
		return
	}

	updt := ig.UpdateStart()
	sort.Slice(ig.DraggedIdxs, func(i, j int) bool {
		return ig.DraggedIdxs[i] > ig.DraggedIdxs[j]
	})
	idx := ig.DraggedIdxs[0]
	for _, i := range ig.DraggedIdxs {
		ig.ImageDeleteAt(i)
	}
	ig.DraggedIdxs = nil
	ig.UpdateEnd(updt)
	ig.SelectIdxAction(idx, mouse.SelectOne)
}

// SaveDraggedIdxs saves selectedindexes into dragged indexes
// taking into account insertion at idx
func (ig *ImgGrid) SaveDraggedIdxs(idx int) {
	sz := len(ig.SelectedIdxs)
	if sz == 0 {
		ig.DraggedIdxs = nil
		return
	}
	ixs := ig.SelectedIdxsList(false) // ascending
	ig.DraggedIdxs = make([]int, len(ixs))
	for i, ix := range ixs {
		if ix > idx {
			ig.DraggedIdxs[i] = ix + sz // make room for insertion
		} else {
			ig.DraggedIdxs[i] = ix
		}
	}
}

// DropBefore inserts object(s) from mime data before this node
func (ig *ImgGrid) DropBefore(md mimedata.Mimes, mod dnd.DropMods, idx int) {
	ig.SaveDraggedIdxs(idx)
	ig.PasteAtIdx(md, idx)
	ig.DragNDropFinalize(mod)
}

// DropAfter inserts object(s) from mime data after this node
func (ig *ImgGrid) DropAfter(md mimedata.Mimes, mod dnd.DropMods, idx int) {
	ig.SaveDraggedIdxs(idx + 1)
	ig.PasteAtIdx(md, idx+1)
	ig.DragNDropFinalize(mod)
}

// DropCancel cancels the drop action e.g., preventing deleting of source
// items in a Move case
func (ig *ImgGrid) DropCancel() {
	ig.DragNDropFinalize(dnd.DropIgnore)
}

//////////////////////////////////////////////////////////////////////////////
//    Events

func (ig *ImgGrid) StdCtxtMenu(m *gi.Menu, idx int) {
	m.AddAction(gi.ActOpts{Label: "Info", Data: idx},
		ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			if igg.InfoFunc != nil {
				igg.InfoFunc(data.(int))
			}
		})
	m.AddAction(gi.ActOpts{Label: "Copy", Data: idx},
		ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			igg.CopyIdxs(true)
		})
	m.AddAction(gi.ActOpts{Label: "Cut", Data: idx},
		ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			igg.CutIdxs()
		})
	m.AddAction(gi.ActOpts{Label: "Paste", Data: idx},
		ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			igg.PasteIdx(data.(int))
		})
	m.AddAction(gi.ActOpts{Label: "Duplicate", Data: idx},
		ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			igg.Duplicate()
		})
	m.AddAction(gi.ActOpts{Label: "Delete", Data: idx},
		ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.Embed(KiT_ImgGrid).(*ImgGrid)
			igg.CutIdxs()
		})
}

func (ig *ImgGrid) ItemCtxtMenu(idx int) {
	var men gi.Menu
	ig.StdCtxtMenu(&men, idx)
	if len(men) > 0 {
		pos := ig.IdxPos(idx)
		gi.PopupMenu(men, pos.X, pos.Y, ig.Viewport, ig.Nm+"-menu")
	}
}

func (ig *ImgGrid) KeyInputActive(kt *key.ChordEvent) {
	if gi.KeyEventTrace {
		fmt.Printf("ImgGrid KeyInput: %v\n", ig.PathUnique())
	}
	kf := gi.KeyFun(kt.Chord())
	selMode := mouse.SelectModeBits(kt.Modifiers)
	if selMode == mouse.SelectOne {
		if ig.SelectMode {
			selMode = mouse.ExtendContinuous
		}
	}
	idx := ig.SelectedIdx
	switch kf {
	case gi.KeyFunCancelSelect:
		ig.UnselectAllIdxs()
		ig.SelectMode = false
		kt.SetProcessed()
	case gi.KeyFunMoveDown:
		ig.MoveDownAction(selMode)
		kt.SetProcessed()
	case gi.KeyFunMoveUp:
		ig.MoveUpAction(selMode)
		kt.SetProcessed()
	case gi.KeyFunPageDown:
		ig.MovePageDownAction(selMode)
		kt.SetProcessed()
	case gi.KeyFunPageUp:
		ig.MovePageUpAction(selMode)
		kt.SetProcessed()
	case gi.KeyFunSelectMode:
		ig.SelectMode = !ig.SelectMode
		kt.SetProcessed()
	case gi.KeyFunSelectAll:
		ig.SelectAllIdxs()
		ig.SelectMode = false
		kt.SetProcessed()
	case gi.KeyFunDelete:
		ig.CutIdxs()
		ig.SelectMode = false
		kt.SetProcessed()
	case gi.KeyFunDuplicate:
		nidx := ig.Duplicate()
		ig.SelectMode = false
		if nidx >= 0 {
			ig.SelectIdxAction(nidx, mouse.SelectOne)
		}
		kt.SetProcessed()
	case gi.KeyFunInsert:
		ig.ImageInsertAt(idx, []string{""})
		ig.SelectMode = false
		ig.SelectIdxAction(idx+1, mouse.SelectOne) // todo: somehow nidx not working
		kt.SetProcessed()
	case gi.KeyFunInsertAfter:
		ig.ImageInsertAt(idx+1, []string{""})
		ig.SelectMode = false
		ig.SelectIdxAction(idx+1, mouse.SelectOne)
		kt.SetProcessed()
	case gi.KeyFunCopy:
		ig.CopyIdxs(true)
		ig.SelectMode = false
		ig.SelectIdxAction(idx, mouse.SelectOne)
		kt.SetProcessed()
	case gi.KeyFunCut:
		ig.CutIdxs()
		ig.SelectMode = false
		kt.SetProcessed()
	case gi.KeyFunPaste:
		ig.PasteIdx(ig.SelectedIdx)
		ig.SelectMode = false
		kt.SetProcessed()
	}
}

var ImgGridProps = ki.Props{
	"EnumType:Flag":    gi.KiT_NodeFlags,
	"background-color": &gi.Prefs.Colors.Background,
	"color":            &gi.Prefs.Colors.Font,
	"border-width":     units.NewPx(2),
	"max-width":        -1,
	"max-height":       -1,
}
