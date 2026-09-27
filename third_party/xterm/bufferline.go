package xterm

// Ported from xterm.js src/common/buffer/BufferLine.ts.
// Each cell occupies 3 uint32 slots: content, fg, bg.

const (
	cellSize    = 3
	cellContent = 0
	cellFg      = 1
	cellBg      = 2

	// cleanupThreshold controls when shrink triggers memory cleanup.
	cleanupThreshold = 2
)

// BufferLine stores a single terminal line as a flat []uint32 with 3 values per cell.
//
// A line in the scrollback keeps only its cells up to the last one that was
// written (see Compact), so data can hold fewer than Len cells; a cell past
// them reads as a null cell, and the first write restores the rest. The two
// maps are nil until a cell needs an entry in them. (Kvit's change; see
// KVIT-PATCH.md.)
type BufferLine struct {
	data          []uint32
	combined      map[int]string
	extendedAttrs map[int]*ExtendedAttrs
	Len           int
	IsWrapped     bool
}

// NewBufferLine creates a BufferLine with cols cells, filled with fillCell.
// If fillCell is nil, cells are filled with null cell defaults.
func NewBufferLine(cols int, fillCell *CellData, isWrapped bool) *BufferLine {
	bl := &BufferLine{
		data:      make([]uint32, cols*cellSize),
		Len:       cols,
		IsWrapped: isWrapped,
	}
	if fillCell == nil {
		fillCell = CellDataFromCharData(NewCharData(0, NullCellChar, NullCellWidth, NullCellCode))
	}
	for i := range cols {
		bl.SetCell(i, fillCell)
	}
	return bl
}

// compactLines is false only in the test that compares a terminal whose
// lines are compacted with one whose lines are not.
var compactLines = true

// nullContent is the content word of a cell nothing was written to.
const nullContent = uint32(NullCellWidth) << ContentWidthShift

// word is one of the three values of a cell, reading a cell past those
// stored as a null cell.
func (bl *BufferLine) word(i int) uint32 {
	if i < len(bl.data) {
		return bl.data[i]
	}
	if i%cellSize == cellContent {
		return nullContent
	}
	return 0
}

// isNullCell reports a cell nothing was written to: no content, width one,
// default colours and no attributes.
func isNullCell(content, fg, bg uint32) bool {
	return content == nullContent && fg == 0 && bg == 0
}

// expand stores every cell of a compacted line again, before a write. It is
// small enough to be inlined into every write; grow does the work.
func (bl *BufferLine) expand() {
	if len(bl.data) < bl.Len*cellSize {
		bl.grow()
	}
}

func (bl *BufferLine) grow() {
	n := bl.Len * cellSize
	stored := len(bl.data)
	if cap(bl.data) >= n {
		bl.data = bl.data[:n]
	} else {
		grown := make([]uint32, n)
		copy(grown, bl.data)
		bl.data = grown
	}
	for i := stored; i < n; i += cellSize {
		bl.data[i+cellContent] = nullContent
		bl.data[i+cellFg] = 0
		bl.data[i+cellBg] = 0
	}
}

// Compact stops storing the null cells at the end of the line, and frees
// maps that hold nothing. A line leaving the screen for the scrollback is
// compacted: a one-word line in a wide terminal then costs the word rather
// than every cell of the width. Blanks a program wrote, and cells with a
// colour or attribute, are kept. (Kvit's addition; see KVIT-PATCH.md.)
func (bl *BufferLine) Compact() { bl.compact() }

// compact is Compact, returning the storage it replaced for a new line to
// reuse, or nil.
func (bl *BufferLine) compact() (released []uint32) {
	if !compactLines {
		return nil
	}
	n := min(len(bl.data)/cellSize, bl.Len)
	for n > 0 {
		i := (n - 1) * cellSize
		if !isNullCell(bl.data[i+cellContent], bl.data[i+cellFg], bl.data[i+cellBg]) {
			break
		}
		n--
	}
	if n*cellSize < len(bl.data) {
		kept := make([]uint32, n*cellSize)
		copy(kept, bl.data)
		released = bl.data
		bl.data = kept
	}
	if len(bl.combined) == 0 {
		bl.combined = nil
	}
	if len(bl.extendedAttrs) == 0 {
		bl.extendedAttrs = nil
	}
	return released
}

// reuse gives the line storage for n cells from spare when spare is large
// enough, so a new line need not allocate. (Kvit's addition.)
func (bl *BufferLine) reuse(spare []uint32, n int) bool {
	if cap(spare) < n*cellSize {
		return false
	}
	bl.data = spare[:n*cellSize]
	return true
}

// StoredCells is how many cells the line stores; the rest read as null
// cells. (Kvit's addition, for tests; see KVIT-PATCH.md.)
func (bl *BufferLine) StoredCells() int { return len(bl.data) / cellSize }

func (bl *BufferLine) setCombined(index int, s string) {
	if bl.combined == nil {
		bl.combined = make(map[int]string)
	}
	bl.combined[index] = s
}

func (bl *BufferLine) setExtended(index int, e *ExtendedAttrs) {
	if bl.extendedAttrs == nil {
		bl.extendedAttrs = make(map[int]*ExtendedAttrs)
	}
	bl.extendedAttrs[index] = e
}

// --- Primitive getters ---

// GetWidth returns the display width of the cell at index.
func (bl *BufferLine) GetWidth(index int) int {
	return int(bl.word(index*cellSize+cellContent) >> ContentWidthShift)
}

// HasWidth returns non-zero if the cell at index has a width set.
func (bl *BufferLine) HasWidth(index int) uint32 {
	return bl.word(index*cellSize+cellContent) & ContentWidthMask
}

// GetFg returns the fg attribute of the cell at index.
func (bl *BufferLine) GetFg(index int) uint32 {
	return bl.word(index*cellSize+cellFg)
}

// GetBg returns the bg attribute of the cell at index.
func (bl *BufferLine) GetBg(index int) uint32 {
	return bl.word(index*cellSize+cellBg)
}

// HasContent returns non-zero if the cell at index has content.
func (bl *BufferLine) HasContent(index int) uint32 {
	return bl.word(index*cellSize+cellContent) & ContentHasContentMask
}

// GetCodePoint returns the codepoint of the cell at index.
// For combined cells, returns the last char code of the combined string.
func (bl *BufferLine) GetCodePoint(index int) uint32 {
	content := bl.word(index*cellSize+cellContent)
	if content&ContentIsCombinedMask != 0 {
		s := bl.combined[index]
		if len(s) == 0 {
			return 0
		}
		// Return last byte as charCode (matching xterm.js charCodeAt behavior for BMP)
		runes := []rune(s)
		return uint32(runes[len(runes)-1])
	}
	return content & ContentCodepointMask
}

// IsCombined returns non-zero if the cell at index has combined content.
func (bl *BufferLine) IsCombined(index int) uint32 {
	return bl.word(index*cellSize+cellContent) & ContentIsCombinedMask
}

// GetString returns the string content of the cell at index.
func (bl *BufferLine) GetString(index int) string {
	content := bl.word(index*cellSize+cellContent)
	if content&ContentIsCombinedMask != 0 {
		return bl.combined[index]
	}
	cp := content & ContentCodepointMask
	if cp != 0 {
		return string(rune(cp))
	}
	return ""
}

// IsProtected returns non-zero if the cell at index has the PROTECTED flag.
func (bl *BufferLine) IsProtected(index int) uint32 {
	return bl.word(index*cellSize+cellBg) & BgFlagProtected
}

// --- Get/Set (legacy CharData) ---

// Get returns the cell at index as a legacy CharData tuple.
func (bl *BufferLine) Get(index int) CharData {
	content := bl.word(index*cellSize+cellContent)
	cp := content & ContentCodepointMask
	var ch string
	if content&ContentIsCombinedMask != 0 {
		ch = bl.combined[index]
	} else if cp != 0 {
		ch = string(rune(cp))
	}
	var code uint32
	if content&ContentIsCombinedMask != 0 {
		runes := []rune(bl.combined[index])
		if len(runes) > 0 {
			code = uint32(runes[len(runes)-1])
		}
	} else {
		code = cp
	}
	return NewCharData(
		bl.word(index*cellSize+cellFg),
		ch,
		int(content>>ContentWidthShift),
		code,
	)
}

// Set sets the cell at index from a legacy CharData tuple.
func (bl *BufferLine) Set(index int, value CharData) {
	bl.expand()
	bl.data[index*cellSize+cellFg] = CharDataAttr(value)
	ch := CharDataChar(value)
	width := CharDataWidth(value)
	runes := []rune(ch)
	if len(runes) > 1 {
		bl.setCombined(index, ch)
		bl.data[index*cellSize+cellContent] = ContentIsCombinedMask | (uint32(width) << ContentWidthShift)
	} else if len(runes) == 1 {
		delete(bl.combined, index)
		bl.data[index*cellSize+cellContent] = uint32(runes[0]) | (uint32(width) << ContentWidthShift)
	} else {
		delete(bl.combined, index)
		bl.data[index*cellSize+cellContent] = uint32(width) << ContentWidthShift
	}
}

// --- Cell-level operations ---

// LoadCell loads the cell at index into the provided CellData, returning it.
func (bl *BufferLine) LoadCell(index int, cell *CellData) *CellData {
	si := index * cellSize
	cell.Content = bl.word(si+cellContent)
	cell.Fg = bl.word(si+cellFg)
	cell.Bg = bl.word(si+cellBg)
	if cell.Content&ContentIsCombinedMask != 0 {
		cell.CombinedData = bl.combined[index]
	} else {
		cell.CombinedData = ""
	}
	cell.Extended = bl.GetExtended(index)
	return cell
}

// GetExtended returns the extended attributes of the cell at index.
func (bl *BufferLine) GetExtended(index int) *ExtendedAttrs {
	if bl.word(index*cellSize+cellBg)&BgFlagHasExtended != 0 {
		return bl.extendedAttrs[index]
	}
	return &ExtendedAttrs{}
}

// SetCell sets the cell at index from a CellData.
func (bl *BufferLine) SetCell(index int, cell *CellData) {
	bl.expand()
	if cell.Content&ContentIsCombinedMask != 0 {
		bl.setCombined(index, cell.CombinedData)
	} else {
		delete(bl.combined, index)
	}
	if cell.Bg&BgFlagHasExtended != 0 {
		bl.setExtended(index, cell.Extended)
	} else {
		delete(bl.extendedAttrs, index)
	}
	si := index * cellSize
	bl.data[si+cellContent] = cell.Content
	bl.data[si+cellFg] = cell.Fg
	bl.data[si+cellBg] = cell.Bg
}

// SetCellFromCodepoint sets a cell from a codepoint, width, and attribute data.
func (bl *BufferLine) SetCellFromCodepoint(index int, codePoint uint32, width int, attrs *AttributeData) {
	bl.expand()
	if len(bl.combined) > 0 {
		delete(bl.combined, index)
	}
	if attrs.Bg&BgFlagHasExtended != 0 {
		bl.setExtended(index, attrs.Extended)
	} else if len(bl.extendedAttrs) > 0 {
		delete(bl.extendedAttrs, index)
	}
	si := index * cellSize
	bl.data[si+cellContent] = codePoint | (uint32(width) << ContentWidthShift)
	bl.data[si+cellFg] = attrs.Fg
	bl.data[si+cellBg] = attrs.Bg
}

// AddCodepointToCell adds a combining codepoint to the cell at index.
func (bl *BufferLine) AddCodepointToCell(index int, codePoint uint32, width int) {
	bl.expand()
	content := bl.data[index*cellSize+cellContent]
	if content&ContentIsCombinedMask != 0 {
		bl.setCombined(index, bl.combined[index]+string(rune(codePoint)))
	} else {
		cp := content & ContentCodepointMask
		if cp != 0 {
			bl.setCombined(index, string(rune(cp))+string(rune(codePoint)))
			content &= ^ContentCodepointMask
			content |= ContentIsCombinedMask
		} else {
			content = codePoint | (1 << ContentWidthShift)
		}
	}
	if width > 0 {
		content &= ^ContentWidthMask
		content |= uint32(width) << ContentWidthShift
	}
	bl.data[index*cellSize+cellContent] = content
}

// --- Bulk operations ---

// InsertCells inserts n cells at pos, shifting existing cells right.
// Cells that fall off the end are lost. New cells are filled with fillCell.
func (bl *BufferLine) InsertCells(pos, n int, fillCell *CellData) {
	pos %= bl.Len
	// Handle fullwidth at pos: reset cell to the left if pos is second cell of a wide char
	if pos > 0 && bl.GetWidth(pos-1) == 2 {
		bl.SetCellFromCodepoint(pos-1, 0, 1, &fillCell.AttributeData)
	}
	if n < bl.Len-pos {
		var tmp CellData
		for i := bl.Len - pos - n - 1; i >= 0; i-- {
			bl.SetCell(pos+n+i, bl.LoadCell(pos+i, &tmp))
		}
		for i := range n {
			bl.SetCell(pos+i, fillCell)
		}
	} else {
		for i := pos; i < bl.Len; i++ {
			bl.SetCell(i, fillCell)
		}
	}
	// Handle fullwidth at line end
	if bl.GetWidth(bl.Len-1) == 2 {
		bl.SetCellFromCodepoint(bl.Len-1, 0, 1, &fillCell.AttributeData)
	}
}

// DeleteCells deletes n cells at pos, shifting remaining cells left.
// Vacated cells at the end are filled with fillCell.
func (bl *BufferLine) DeleteCells(pos, n int, fillCell *CellData) {
	pos %= bl.Len
	if n < bl.Len-pos {
		var tmp CellData
		for i := range bl.Len - pos - n {
			bl.SetCell(pos+i, bl.LoadCell(pos+n+i, &tmp))
		}
		for i := bl.Len - n; i < bl.Len; i++ {
			bl.SetCell(i, fillCell)
		}
	} else {
		for i := pos; i < bl.Len; i++ {
			bl.SetCell(i, fillCell)
		}
	}
	// Handle fullwidth at pos
	if pos > 0 && bl.GetWidth(pos-1) == 2 {
		bl.SetCellFromCodepoint(pos-1, 0, 1, &fillCell.AttributeData)
	}
	if bl.GetWidth(pos) == 0 && bl.HasContent(pos) == 0 {
		bl.SetCellFromCodepoint(pos, 0, 1, &fillCell.AttributeData)
	}
}

// ReplaceCells replaces cells in [start, end) with fillCell.
func (bl *BufferLine) ReplaceCells(start, end int, fillCell *CellData, respectProtect bool) {
	if respectProtect {
		if start > 0 && bl.GetWidth(start-1) == 2 && bl.IsProtected(start-1) == 0 {
			bl.SetCellFromCodepoint(start-1, 0, 1, &fillCell.AttributeData)
		}
		if end < bl.Len && bl.GetWidth(end-1) == 2 && bl.IsProtected(end) == 0 {
			bl.SetCellFromCodepoint(end, 0, 1, &fillCell.AttributeData)
		}
		for start < end && start < bl.Len {
			if bl.IsProtected(start) == 0 {
				bl.SetCell(start, fillCell)
			}
			start++
		}
		return
	}
	// Handle fullwidth at start
	if start > 0 && bl.GetWidth(start-1) == 2 {
		bl.SetCellFromCodepoint(start-1, 0, 1, &fillCell.AttributeData)
	}
	// Handle fullwidth at end
	if end < bl.Len && bl.GetWidth(end-1) == 2 {
		bl.SetCellFromCodepoint(end, 0, 1, &fillCell.AttributeData)
	}
	for start < end && start < bl.Len {
		bl.SetCell(start, fillCell)
		start++
	}
}

// Resize resizes the line to cols cells, filling new cells with fillCell.
// Returns true if a CleanupMemory call would free excess memory.
func (bl *BufferLine) Resize(cols int, fillCell *CellData) bool {
	if cols == bl.Len {
		return len(bl.data)*4*cleanupThreshold < cap(bl.data)*4
	}
	uint32Cells := cols * cellSize
	if len(bl.data) < bl.Len*cellSize {
		// A compacted line: the null cells it does not store stretch or
		// shrink with it, unless it grows with cells of another kind.
		if cols > bl.Len && !isNullCell(fillCell.Content, fillCell.Fg, fillCell.Bg) {
			bl.expand()
		} else {
			if len(bl.data) > uint32Cells {
				bl.data = bl.data[:uint32Cells]
			}
			for k := range bl.combined {
				if k >= cols {
					delete(bl.combined, k)
				}
			}
			for k := range bl.extendedAttrs {
				if k >= cols {
					delete(bl.extendedAttrs, k)
				}
			}
			bl.Len = cols
			return false
		}
	}
	if cols > bl.Len {
		if cap(bl.data) >= uint32Cells {
			bl.data = bl.data[:uint32Cells]
		} else {
			newData := make([]uint32, uint32Cells)
			copy(newData, bl.data)
			bl.data = newData
		}
		for i := bl.Len; i < cols; i++ {
			bl.SetCell(i, fillCell)
		}
	} else {
		bl.data = bl.data[:uint32Cells]
		// Remove combined data and extended attrs beyond new length
		for k := range bl.combined {
			if k >= cols {
				delete(bl.combined, k)
			}
		}
		for k := range bl.extendedAttrs {
			if k >= cols {
				delete(bl.extendedAttrs, k)
			}
		}
	}
	bl.Len = cols
	return uint32Cells*4*cleanupThreshold < cap(bl.data)*4
}

// CleanupMemory reallocates the backing array if it exceeds the threshold.
// Returns 1 if cleanup happened, 0 otherwise.
func (bl *BufferLine) CleanupMemory() int {
	if len(bl.data)*4*cleanupThreshold < cap(bl.data)*4 {
		newData := make([]uint32, len(bl.data))
		copy(newData, bl.data)
		bl.data = newData
		return 1
	}
	return 0
}

// Fill fills all cells with fillCell.
func (bl *BufferLine) Fill(fillCell *CellData, respectProtect bool) {
	if respectProtect {
		for i := range bl.Len {
			if bl.IsProtected(i) == 0 {
				bl.SetCell(i, fillCell)
			}
		}
		return
	}
	bl.combined = nil
	bl.extendedAttrs = nil
	for i := range bl.Len {
		bl.SetCell(i, fillCell)
	}
}

// CopyFrom copies all data from another BufferLine.
func (bl *BufferLine) CopyFrom(line *BufferLine) {
	if len(bl.data) != len(line.data) {
		bl.data = make([]uint32, len(line.data))
	}
	copy(bl.data, line.data)
	bl.Len = line.Len
	bl.copySparseMapsFrom(line)
	bl.IsWrapped = line.IsWrapped
}

// Clone returns a deep copy of the BufferLine.
func (bl *BufferLine) Clone() *BufferLine {
	newLine := &BufferLine{
		data:      make([]uint32, len(bl.data)),
		Len:       bl.Len,
		IsWrapped: bl.IsWrapped,
	}
	copy(newLine.data, bl.data)
	newLine.copySparseMapsFrom(bl)
	return newLine
}

// --- Query ---

// GetTrimmedLength returns the number of columns with content, accounting for wide chars.
func (bl *BufferLine) GetTrimmedLength() int {
	for i := bl.Len - 1; i >= 0; i-- {
		if bl.word(i*cellSize+cellContent)&ContentHasContentMask != 0 {
			return i + int(bl.word(i*cellSize+cellContent)>>ContentWidthShift)
		}
	}
	return 0
}

// GetNoBgTrimmedLength returns the trimmed length considering both content and bg color.
func (bl *BufferLine) GetNoBgTrimmedLength() int {
	for i := bl.Len - 1; i >= 0; i-- {
		if bl.word(i*cellSize+cellContent)&ContentHasContentMask != 0 ||
			bl.word(i*cellSize+cellBg)&AttrCMMask != 0 {
			return i + int(bl.word(i*cellSize+cellContent)>>ContentWidthShift)
		}
	}
	return 0
}

// CopyCellsFrom copies length cells from src starting at srcCol to bl starting at destCol.
func (bl *BufferLine) CopyCellsFrom(src *BufferLine, srcCol, destCol, length int, applyInReverse bool) {
	bl.expand()
	if applyInReverse {
		for cell := length - 1; cell >= 0; cell-- {
			for i := range cellSize {
				bl.data[(destCol+cell)*cellSize+i] = src.word((srcCol+cell)*cellSize + i)
			}
			bl.copyCellMapsFrom(src, srcCol+cell, destCol+cell)
		}
	} else {
		for cell := range length {
			for i := range cellSize {
				bl.data[(destCol+cell)*cellSize+i] = src.word((srcCol+cell)*cellSize + i)
			}
			bl.copyCellMapsFrom(src, srcCol+cell, destCol+cell)
		}
	}
}

// copyCellMapsFrom copies the sparse combined and extended attribute entries for a
// single cell from src[srcCol] to bl[destCol]. It uses the source cell flags to
// decide whether a sparse entry exists, so only the requested cells are touched.
func (bl *BufferLine) copyCellMapsFrom(src *BufferLine, srcCol, destCol int) {
	if src.word(srcCol*cellSize+cellContent)&ContentIsCombinedMask != 0 {
		bl.setCombined(destCol, src.combined[srcCol])
	} else {
		delete(bl.combined, destCol)
	}
	if src.word(srcCol*cellSize+cellBg)&BgFlagHasExtended != 0 {
		bl.setExtended(destCol, src.extendedAttrs[srcCol])
	} else {
		delete(bl.extendedAttrs, destCol)
	}
}

func (bl *BufferLine) copySparseMapsFrom(src *BufferLine) {
	bl.combined = nil
	bl.extendedAttrs = nil
	if len(src.combined) == 0 && len(src.extendedAttrs) == 0 {
		return
	}
	for i := range min(src.Len, len(src.data)/cellSize) {
		si := i * cellSize
		if src.data[si+cellContent]&ContentIsCombinedMask != 0 {
			bl.setCombined(i, src.combined[i])
		}
		if src.data[si+cellBg]&BgFlagHasExtended != 0 {
			bl.setExtended(i, src.extendedAttrs[i])
		}
	}
}

// TranslateToString converts the line to a string.
// If trimRight is true, trailing empty cells are excluded.
// startCol and endCol define the range (endCol is exclusive, -1 means bl.Len).
func (bl *BufferLine) TranslateToString(trimRight bool, startCol, endCol int) string {
	if endCol < 0 {
		endCol = bl.Len
	}
	if trimRight {
		tl := bl.GetTrimmedLength()
		if tl < endCol {
			endCol = tl
		}
	}
	result := make([]byte, 0, endCol-startCol)
	for startCol < endCol {
		content := bl.word(startCol*cellSize+cellContent)
		cp := content & ContentCodepointMask
		var chars string
		if content&ContentIsCombinedMask != 0 {
			chars = bl.combined[startCol]
		} else if cp != 0 {
			chars = string(rune(cp))
		} else {
			chars = WhitespaceCellChar
		}
		result = append(result, chars...)
		w := int(content >> ContentWidthShift)
		if w == 0 {
			w = 1
		}
		startCol += w
	}
	return string(result)
}
