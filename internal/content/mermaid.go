package content

// Native Mermaid flowchart renderer.
//
// Supports a useful subset:
//   - `graph TD|TB|BT|LR|RL` and `flowchart` aliases
//   - Nodes: A, A[Label], A(Label), A([Label]), A{Label}, A((Label))
//   - Edges: A --> B, A --- B, A -.-> B, A ==> B, with chains A --> B --> C
//   - Edge labels: A -->|text| B  or  A -- text --> B
//   - Comments: lines starting with %%
//
// Layout: longest-path level assignment + simple per-level horizontal packing.
// Rendering: box-drawing characters; arrows are straight or single-elbow.
// Falls back (returns ok=false) for unsupported diagram types.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

type mermShape int

const (
	shapeRect mermShape = iota
	shapeRound
	shapeStadium
	shapeDiamond
	shapeCircle
)

type mermNode struct {
	id    string
	label string
	shape mermShape

	level int
	x, y  int // grid top-left
	w, h  int
}

type mermEdge struct {
	from, to string
	label    string
}

type mermGraph struct {
	dir   string // TD, BT, LR, RL
	nodes map[string]*mermNode
	order []string
	edges []mermEdge
}

func (g *mermGraph) ensureNode(n *mermNode) *mermNode {
	if existing, ok := g.nodes[n.id]; ok {
		// Upgrade label/shape if the new ref has explicit ones and existing doesn't.
		if existing.label == existing.id && n.label != n.id {
			existing.label = n.label
			existing.shape = n.shape
		}
		return existing
	}
	g.nodes[n.id] = n
	g.order = append(g.order, n.id)
	return n
}

// --- entry: native render ---

func tryNativeMermaid(src string) (string, bool) {
	g, err := parseMermaid(src)
	if err != nil || g == nil {
		return "", false
	}
	if len(g.nodes) == 0 {
		return "", false
	}
	return renderGraph(g), true
}

// --- parser ---

var (
	idRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
	commentRe = regexp.MustCompile(`%%.*`)
	// Edge operators we recognize, in order so longer ones win.
	edgeOps = []string{"-.->", "==>", "-->", "---", "==="}
)

func parseMermaid(src string) (*mermGraph, error) {
	g := &mermGraph{nodes: map[string]*mermNode{}}
	lines := strings.Split(src, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(commentRe.ReplaceAllString(raw, ""))
		if line == "" {
			continue
		}
		if g.dir == "" {
			parts := strings.Fields(line)
			if len(parts) >= 2 && (parts[0] == "graph" || parts[0] == "flowchart") {
				dir := strings.ToUpper(parts[1])
				switch dir {
				case "TD", "TB", "BT", "LR", "RL":
					g.dir = dir
				default:
					return nil, fmt.Errorf("unsupported direction %q", dir)
				}
				// allow nodes/edges to follow on same line after direction
				rest := strings.TrimSpace(strings.Join(parts[2:], " "))
				if rest != "" {
					parseLine(rest, g)
				}
				continue
			}
			return nil, fmt.Errorf("expected 'graph TD' header")
		}
		parseLine(line, g)
	}
	if g.dir == "" {
		return nil, fmt.Errorf("no graph header")
	}
	return g, nil
}

func parseLine(line string, g *mermGraph) {
	// Subgraph / style / classDef etc — skip silently.
	first := strings.Fields(line)
	if len(first) > 0 {
		switch first[0] {
		case "subgraph", "end", "style", "classDef", "class", "click", "linkStyle":
			return
		}
	}
	if isEdgeLine(line) {
		parseEdgeChain(line, g)
		return
	}
	// Standalone node declaration: e.g. `A[Label]`
	if n, _ := parseNodeRef(line); n != nil {
		g.ensureNode(n)
	}
}

func isEdgeLine(s string) bool {
	for _, op := range edgeOps {
		if strings.Contains(s, op) {
			return true
		}
	}
	return false
}

// findNextEdge returns the byte index, length, and operator of the next edge
// op in s, or -1 if none.
func findNextEdge(s string) (idx, oplen int) {
	best := -1
	bestLen := 0
	for _, op := range edgeOps {
		if i := strings.Index(s, op); i >= 0 && (best == -1 || i < best || (i == best && len(op) > bestLen)) {
			best = i
			bestLen = len(op)
		}
	}
	return best, bestLen
}

func parseEdgeChain(line string, g *mermGraph) {
	rest := line
	var prev *mermNode
	for {
		idx, oplen := findNextEdge(rest)
		if idx < 0 {
			if prev != nil {
				if n, _ := parseNodeRef(strings.TrimSpace(rest)); n != nil {
					n = g.ensureNode(n)
					g.edges = append(g.edges, mermEdge{prev.id, n.id, ""})
				}
			}
			return
		}
		left := strings.TrimSpace(rest[:idx])
		after := strings.TrimSpace(rest[idx+oplen:])

		// Optional edge label: -- text --> or -->|text|
		var edgeLabel string
		if strings.HasPrefix(after, "|") {
			if end := strings.Index(after[1:], "|"); end >= 0 {
				edgeLabel = strings.TrimSpace(after[1 : 1+end])
				after = strings.TrimSpace(after[1+end+1:])
			}
		}

		var srcNode *mermNode
		if prev != nil && left == "" {
			srcNode = prev
		} else if n, _ := parseNodeRef(left); n != nil {
			srcNode = g.ensureNode(n)
		}

		// Parse the leading node of `after` so we can attach the edge and
		// continue scanning the rest of the chain.
		nextRest := after
		if n, leftover := parseNodeRef(after); n != nil {
			nextRest = leftover
			dst := g.ensureNode(n)
			if srcNode != nil {
				g.edges = append(g.edges, mermEdge{srcNode.id, dst.id, edgeLabel})
			}
			prev = dst
		} else {
			// Neither side of the operator yielded a parseable node — bail
			// instead of looping forever on the same input.
			return
		}
		rest = strings.TrimSpace(nextRest)
		if rest == "" {
			return
		}
	}
}

// parseNodeRef parses a leading node reference like "A", "A[Label]", "A(Foo)".
// Returns the node and the remainder of the string after the ref.
func parseNodeRef(s string) (*mermNode, string) {
	s = strings.TrimSpace(s)
	loc := idRe.FindStringIndex(s)
	if loc == nil {
		return nil, s
	}
	id := s[:loc[1]]
	rest := s[loc[1]:]
	n := &mermNode{id: id, label: id, shape: shapeRect}

	// Try shape brackets in order (longest first to avoid overlap).
	type spec struct {
		open, close string
		shape       mermShape
	}
	specs := []spec{
		{"((", "))", shapeCircle},
		{"([", "])", shapeStadium},
		{"[[", "]]", shapeStadium},
		{"[", "]", shapeRect},
		{"(", ")", shapeRound},
		{"{", "}", shapeDiamond},
	}
	for _, sp := range specs {
		if strings.HasPrefix(rest, sp.open) {
			end := strings.Index(rest[len(sp.open):], sp.close)
			if end >= 0 {
				label := rest[len(sp.open) : len(sp.open)+end]
				label = strings.Trim(label, `"`)
				n.label = label
				n.shape = sp.shape
				rest = rest[len(sp.open)+end+len(sp.close):]
				return n, rest
			}
		}
	}
	return n, rest
}

// --- layout ---

func layoutGraph(g *mermGraph) (rows int, cols int) {
	// Box sizes.
	for _, n := range g.nodes {
		w := runewidth.StringWidth(n.label)
		switch n.shape {
		case shapeDiamond:
			n.w = w + 4
		case shapeRound, shapeStadium:
			n.w = w + 4
		default:
			n.w = w + 4
		}
		if n.w < 5 {
			n.w = 5
		}
		n.h = 3
	}

	// Longest-path levels from roots (no incoming).
	incoming := map[string]int{}
	for _, e := range g.edges {
		incoming[e.to]++
	}
	level := map[string]int{}
	for _, id := range g.order {
		if incoming[id] == 0 {
			level[id] = 0
		}
	}
	// Iterate to stability.
	for iter := 0; iter < len(g.nodes)+2; iter++ {
		changed := false
		for _, e := range g.edges {
			lf, okF := level[e.from]
			if !okF {
				continue
			}
			lt, okT := level[e.to]
			if !okT || lf+1 > lt {
				level[e.to] = lf + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	// Nodes that weren't reachable from any root: assign incrementally.
	for _, id := range g.order {
		if _, ok := level[id]; !ok {
			level[id] = 0
		}
	}

	// Group by level, in declaration order.
	maxLevel := 0
	byLevel := map[int][]*mermNode{}
	for _, id := range g.order {
		l := level[id]
		n := g.nodes[id]
		n.level = l
		byLevel[l] = append(byLevel[l], n)
		if l > maxLevel {
			maxLevel = l
		}
	}

	const hGap = 4 // between boxes in a row
	const vGap = 2 // between rows (for arrows)

	if g.dir == "LR" || g.dir == "RL" {
		// Horizontal layout: each level is a column; nodes stack vertically.
		x := 0
		maxRow := 0
		for l := 0; l <= maxLevel; l++ {
			nodes := byLevel[l]
			colW := 0
			y := 0
			for _, n := range nodes {
				n.x = x
				n.y = y
				y += n.h + vGap
				if n.w > colW {
					colW = n.w
				}
			}
			if y-vGap > maxRow {
				maxRow = y - vGap
			}
			x += colW + hGap + 2 // extra room for arrows
		}
		return maxRow, x - hGap - 2
	}

	// TD (default) / TB / BT: each level is a row; nodes side-by-side.
	y := 0
	maxCol := 0
	for l := 0; l <= maxLevel; l++ {
		nodes := byLevel[l]
		x := 0
		rowH := 0
		for _, n := range nodes {
			n.x = x
			n.y = y
			x += n.w + hGap
			if n.h > rowH {
				rowH = n.h
			}
		}
		if x-hGap > maxCol {
			maxCol = x - hGap
		}
		y += rowH + vGap + 1 // +1 for arrowhead row
	}
	if g.dir == "BT" {
		// Flip vertically.
		for _, n := range g.nodes {
			n.y = (y - vGap - 1) - n.y - n.h
		}
	}
	if g.dir == "RL" {
		for _, n := range g.nodes {
			n.x = (maxCol) - n.x - n.w
		}
	}
	return y - vGap - 1, maxCol
}

// --- render ---

type grid struct {
	cells [][]rune
	w, h  int
}

func newGrid(w, h int) *grid {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	cells := make([][]rune, h)
	for i := range cells {
		cells[i] = make([]rune, w)
		for j := range cells[i] {
			cells[i][j] = ' '
		}
	}
	return &grid{cells: cells, w: w, h: h}
}

func (g *grid) set(x, y int, r rune) {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return
	}
	g.cells[y][x] = r
}

func (g *grid) setStr(x, y int, s string) {
	for _, r := range s {
		g.set(x, y, r)
		x += runewidth.RuneWidth(r)
	}
}

func (g *grid) String() string {
	var sb strings.Builder
	for i, row := range g.cells {
		sb.WriteString(strings.TrimRight(string(row), " "))
		if i < len(g.cells)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func renderGraph(g *mermGraph) string {
	h, w := layoutGraph(g)
	gr := newGrid(w+2, h+1)

	// Draw boxes.
	for _, n := range g.nodes {
		drawBox(gr, n)
	}
	// Draw edges.
	for _, e := range g.edges {
		src := g.nodes[e.from]
		dst := g.nodes[e.to]
		if src == nil || dst == nil {
			continue
		}
		drawEdge(gr, src, dst, e.label, g.dir)
	}
	return gr.String()
}

func drawBox(g *grid, n *mermNode) {
	x, y, w, h := n.x, n.y, n.w, n.h
	switch n.shape {
	case shapeDiamond:
		// Render as { Label } stylized — simple bracket for now.
		g.set(x, y+1, '◇')
		g.setStr(x+2, y+1, n.label)
		g.set(x+w-1, y+1, '◇')
		// top/bottom dashes
		for i := 1; i < w-1; i++ {
			g.set(x+i, y, '·')
			g.set(x+i, y+2, '·')
		}
		return
	case shapeCircle:
		g.set(x, y+1, '(')
		g.setStr(x+2, y+1, n.label)
		g.set(x+w-1, y+1, ')')
		return
	}
	// Rounded box for round/stadium, square for rect.
	tl, tr, bl, br := '┌', '┐', '└', '┘'
	if n.shape == shapeRound || n.shape == shapeStadium {
		tl, tr, bl, br = '╭', '╮', '╰', '╯'
	}
	g.set(x, y, tl)
	g.set(x+w-1, y, tr)
	g.set(x, y+h-1, bl)
	g.set(x+w-1, y+h-1, br)
	for i := 1; i < w-1; i++ {
		g.set(x+i, y, '─')
		g.set(x+i, y+h-1, '─')
	}
	for i := 1; i < h-1; i++ {
		g.set(x, y+i, '│')
		g.set(x+w-1, y+i, '│')
	}
	// Label centered.
	labelW := runewidth.StringWidth(n.label)
	lx := x + (w-labelW)/2
	if lx < x+1 {
		lx = x + 1
	}
	g.setStr(lx, y+1, n.label)
}

func drawEdge(g *grid, src, dst *mermNode, label, dir string) {
	switch dir {
	case "LR", "RL":
		drawHorizEdge(g, src, dst, label, dir == "RL")
	default:
		drawVertEdge(g, src, dst, label, dir == "BT")
	}
}

func drawVertEdge(g *grid, src, dst *mermNode, label string, upward bool) {
	// Connect bottom of src to top of dst.
	srcMidX := src.x + src.w/2
	dstMidX := dst.x + dst.w/2

	var y1, y2 int
	if !upward {
		y1 = src.y + src.h
		y2 = dst.y - 1
		if y2 < y1 {
			return
		}
	} else {
		y1 = src.y - 1
		y2 = dst.y + dst.h
		if y1 < y2 {
			return
		}
	}

	if srcMidX == dstMidX {
		// straight line
		if !upward {
			for y := y1; y < y2; y++ {
				g.set(srcMidX, y, '│')
			}
			g.set(srcMidX, y2, '▼')
		} else {
			for y := y2 + 1; y <= y1; y++ {
				g.set(srcMidX, y, '│')
			}
			g.set(srcMidX, y2, '▲')
		}
		if label != "" {
			lx := srcMidX + 2
			ly := (y1 + y2) / 2
			g.setStr(lx, ly, label)
		}
		return
	}

	// Elbow: down from src to midY, across, then down/up to dst.
	if !upward {
		midY := (y1 + y2) / 2
		// vertical down from src
		for y := y1; y <= midY; y++ {
			g.set(srcMidX, y, '│')
		}
		// horizontal
		x1, x2 := srcMidX, dstMidX
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		for x := x1; x <= x2; x++ {
			g.set(x, midY, '─')
		}
		// corners
		if srcMidX < dstMidX {
			g.set(srcMidX, midY, '╰')
			g.set(dstMidX, midY, '╮')
		} else {
			g.set(srcMidX, midY, '╯')
			g.set(dstMidX, midY, '╭')
		}
		// vertical down to dst
		for y := midY + 1; y < y2; y++ {
			g.set(dstMidX, y, '│')
		}
		g.set(dstMidX, y2, '▼')
		if label != "" {
			g.setStr((srcMidX+dstMidX)/2-len(label)/2, midY-1, label)
		}
	} else {
		midY := (y1 + y2) / 2
		for y := midY; y <= y1; y++ {
			g.set(srcMidX, y, '│')
		}
		x1, x2 := srcMidX, dstMidX
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		for x := x1; x <= x2; x++ {
			g.set(x, midY, '─')
		}
		for y := y2 + 1; y < midY; y++ {
			g.set(dstMidX, y, '│')
		}
		g.set(dstMidX, y2, '▲')
	}
}

// seqAbs returns the absolute value of x.
func seqAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func drawHorizEdge(g *grid, src, dst *mermNode, label string, leftward bool) {
	srcMidY := src.y + src.h/2
	dstMidY := dst.y + dst.h/2

	var x1, x2 int
	if !leftward {
		x1 = src.x + src.w
		x2 = dst.x - 1
		if x2 < x1 {
			return
		}
	} else {
		x1 = src.x - 1
		x2 = dst.x + dst.w
		if x1 < x2 {
			return
		}
	}

	if srcMidY == dstMidY {
		if !leftward {
			for x := x1; x < x2; x++ {
				g.set(x, srcMidY, '─')
			}
			g.set(x2, srcMidY, '▶')
		} else {
			for x := x2 + 1; x <= x1; x++ {
				g.set(x, srcMidY, '─')
			}
			g.set(x2, srcMidY, '◀')
		}
		if label != "" {
			g.setStr((x1+x2)/2-len(label)/2, srcMidY-1, label)
		}
		return
	}

	if !leftward {
		midX := (x1 + x2) / 2
		for x := x1; x <= midX; x++ {
			g.set(x, srcMidY, '─')
		}
		y1, y2 := srcMidY, dstMidY
		if y1 > y2 {
			y1, y2 = y2, y1
		}
		for y := y1; y <= y2; y++ {
			g.set(midX, y, '│')
		}
		if srcMidY < dstMidY {
			g.set(midX, srcMidY, '╮')
			g.set(midX, dstMidY, '╰')
		} else {
			g.set(midX, srcMidY, '╯')
			g.set(midX, dstMidY, '╭')
		}
		for x := midX + 1; x < x2; x++ {
			g.set(x, dstMidY, '─')
		}
		g.set(x2, dstMidY, '▶')
	}
}

// ─────────────────────── sequence diagram ───────────────────────

type seqParticipant struct {
	id    string
	alias string
	idx   int
}

type seqMsg struct {
	from   string
	to     string
	text   string
	dotted bool
}

type seqDiagram struct {
	order []string
	parts map[string]*seqParticipant
	msgs  []seqMsg
}

// Matches: A->>B: text  A-->B: text  A->B: text  A-->>B: text
var seqArrowRe = regexp.MustCompile(`^(\S+?)(-->>|-->|->>|->)(\S+?)\s*:\s*(.*)$`)

func tryNativeSequence(src string) (string, bool) {
	d, err := parseSequence(src)
	if err != nil || d == nil || len(d.parts) == 0 {
		return "", false
	}
	return renderSequence(d), true
}

func parseSequence(src string) (*seqDiagram, error) {
	d := &seqDiagram{parts: map[string]*seqParticipant{}}
	lines := strings.Split(src, "\n")
	foundHeader := false

	addPart := func(id, alias string) {
		if _, ok := d.parts[id]; ok {
			return
		}
		p := &seqParticipant{id: id, alias: alias, idx: len(d.order)}
		d.order = append(d.order, id)
		d.parts[id] = p
	}

	for _, raw := range lines {
		line := strings.TrimSpace(commentRe.ReplaceAllString(raw, ""))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)

		if !foundHeader {
			if lower == "sequencediagram" {
				foundHeader = true
				continue
			}
			return nil, fmt.Errorf("expected sequenceDiagram header")
		}

		// Skip block-structure keywords.
		if f := strings.Fields(line); len(f) > 0 {
			switch strings.ToLower(f[0]) {
			case "loop", "alt", "else", "opt", "par", "critical", "break",
				"end", "note", "activate", "deactivate", "autonumber", "rect":
				continue
			}
		}

		// Participant / actor declaration.
		if strings.HasPrefix(lower, "participant ") || strings.HasPrefix(lower, "actor ") {
			space := strings.IndexByte(line, ' ')
			rest := strings.TrimSpace(line[space+1:])
			id, alias := rest, rest
			if idx := strings.Index(strings.ToLower(rest), " as "); idx >= 0 {
				id = strings.TrimSpace(rest[:idx])
				alias = strings.TrimSpace(rest[idx+4:])
			}
			addPart(id, alias)
			continue
		}

		// Arrow message.
		if m := seqArrowRe.FindStringSubmatch(line); m != nil {
			from, op, to, text := m[1], m[2], m[3], strings.TrimSpace(m[4])
			addPart(from, from)
			addPart(to, to)
			d.msgs = append(d.msgs, seqMsg{
				from:   from,
				to:     to,
				text:   text,
				dotted: strings.HasPrefix(op, "--"),
			})
		}
	}

	if !foundHeader {
		return nil, fmt.Errorf("no sequenceDiagram header")
	}
	return d, nil
}

func renderSequence(d *seqDiagram) string {
	n := len(d.order)
	if n == 0 {
		return ""
	}

	// Participant box widths.
	partW := make([]int, n)
	maxBoxW := 0
	for i, id := range d.order {
		w := runewidth.StringWidth(d.parts[id].alias) + 4
		if w < 7 {
			w = 7
		}
		partW[i] = w
		if w > maxBoxW {
			maxBoxW = w
		}
	}

	// Center-to-center column spacing: large enough that adjacent boxes don't
	// overlap and adjacent message text fits between lifelines.
	colSpacing := maxBoxW + 4
	for _, msg := range d.msgs {
		fi := d.parts[msg.from].idx
		ti := d.parts[msg.to].idx
		if fi == ti {
			continue
		}
		dist := seqAbs(ti - fi)
		need := (runewidth.StringWidth(msg.text) + 4 + dist - 1) / dist
		if need > colSpacing {
			colSpacing = need
		}
	}

	// Participant centers and grid dimensions.
	centers := make([]int, n)
	for i := range centers {
		centers[i] = i*colSpacing + colSpacing/2
	}

	// 3 header + 1 gap + msgs*2 + 1 gap + 3 footer
	headerH := 3
	bodyH := 1 + len(d.msgs)*2 + 1
	footerH := 3
	totalH := headerH + bodyH + footerH
	totalW := centers[n-1] + partW[n-1]/2 + 4

	gr := newGrid(totalW, totalH)

	// Header participant boxes.
	for i, id := range d.order {
		drawSeqBox(gr, centers[i]-partW[i]/2, 0, partW[i], d.parts[id].alias)
	}

	// Lifelines.
	for i := range d.order {
		cx := centers[i]
		for y := headerH; y < headerH+bodyH; y++ {
			gr.set(cx, y, '│')
		}
	}

	// Messages.
	for mi, msg := range d.msgs {
		x1 := centers[d.parts[msg.from].idx]
		x2 := centers[d.parts[msg.to].idx]
		textY := headerH + 1 + mi*2
		arrowY := textY + 1

		// Text label centered in the span.
		if msg.text != "" {
			tw := runewidth.StringWidth(msg.text)
			lo, hi := x1, x2
			if lo > hi {
				lo, hi = hi, lo
			}
			tx := lo + 1 + (hi-lo-1-tw)/2
			if tx < lo+1 {
				tx = lo + 1
			}
			gr.setStr(tx, textY, msg.text)
		}

		lineRune := '─'
		if msg.dotted {
			lineRune = '╌'
		}

		switch {
		case x1 == x2:
			// Self-message: small right-side loop across textY and arrowY.
			gr.set(x1+1, textY, '─')
			gr.set(x1+2, textY, '╮')
			gr.set(x1, arrowY, '◄')
			gr.set(x1+1, arrowY, '─')
			gr.set(x1+2, arrowY, '╯')
		case x1 < x2:
			for x := x1 + 1; x < x2; x++ {
				gr.set(x, arrowY, lineRune)
			}
			gr.set(x2, arrowY, '►')
		default:
			gr.set(x2, arrowY, '◄')
			for x := x2 + 1; x < x1; x++ {
				gr.set(x, arrowY, lineRune)
			}
		}
	}

	// Footer participant boxes.
	footerY := headerH + bodyH
	for i, id := range d.order {
		drawSeqBox(gr, centers[i]-partW[i]/2, footerY, partW[i], d.parts[id].alias)
	}

	return gr.String()
}

func drawSeqBox(g *grid, x, y, w int, label string) {
	g.set(x, y, '┌')
	g.set(x+w-1, y, '┐')
	g.set(x, y+2, '└')
	g.set(x+w-1, y+2, '┘')
	for i := 1; i < w-1; i++ {
		g.set(x+i, y, '─')
		g.set(x+i, y+2, '─')
	}
	g.set(x, y+1, '│')
	g.set(x+w-1, y+1, '│')
	lw := runewidth.StringWidth(label)
	lx := x + (w-lw)/2
	if lx < x+1 {
		lx = x + 1
	}
	g.setStr(lx, y+1, label)
}
