// Simple wrapper for rrdtool C library
package rrd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type Error string

func (e Error) Error() string {
	return string(e)
}

type cstring []byte

func newCstring(s string) cstring {
	cs := make(cstring, len(s)+1)
	copy(cs, s)
	return cs
}

func (cs cstring) p() unsafe.Pointer {
	if len(cs) == 0 {
		return nil
	}
	return unsafe.Pointer(&cs[0])
}

func (cs cstring) String() string {
	return string(cs[:len(cs)-1])
}

func join(args []interface{}) string {
	sa := make([]string, len(args))
	for i, a := range args {
		var s string
		switch v := a.(type) {
		case time.Time:
			s = fmt.Sprint(v.Unix())
		default:
			s = fmt.Sprint(v)
		}
		sa[i] = s
	}
	return strings.Join(sa, ":")
}

type Creator struct {
	filename string
	start    time.Time
	step     uint
	args     []string
}

// NewCreator returns new Creator object. You need to call Create to really
// create database file.
//	filename - name of database file
//	start    - don't accept any data timed before or at time specified
//	step     - base interval in seconds with which data will be fed into RRD
func NewCreator(filename string, start time.Time, step uint) *Creator {
	return &Creator{
		filename: filename,
		start:    start,
		step:     step,
	}
}

func (c *Creator) DS(name, compute string, args ...interface{}) {
	c.args = append(c.args, "DS:"+name+":"+compute+":"+join(args))
}

func (c *Creator) RRA(cf string, args ...interface{}) {
	c.args = append(c.args, "RRA:"+cf+":"+join(args))
}

func (c *Creator) Create(overwrite bool) error {
	if !overwrite {
		f, err := os.OpenFile(
			c.filename,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0666,
		)
		if err != nil {
			return err
		}
		f.Close()
	}
	return c.create()
}

// Use cstring and unsafe.Pointer to avoid alocations for C calls

type Updater struct {
	filename cstring
	template cstring

	args []unsafe.Pointer
}

func NewUpdater(filename string) *Updater {
	return &Updater{filename: newCstring(filename)}
}

func (u *Updater) SetTemplate(dsName ...string) {
	u.template = newCstring(strings.Join(dsName, ":"))
}

// Cache chaches data for later save using Update(). Use it to avoid
// open/read/write/close for every update.
func (u *Updater) Cache(args ...interface{}) {
	u.args = append(u.args, newCstring(join(args)).p())
}

// Update saves data in RRDB.
// Without args Update saves all subsequent updates buffered by Cache method.
// If you specify args it saves them immediately (this is thread-safe
// operation).
func (u *Updater) Update(args ...interface{}) error {
	if len(args) != 0 {
		a := make([]unsafe.Pointer, 1)
		a[0] = newCstring(join(args)).p()
		return u.update(a)
	} else if len(u.args) != 0 {
		err := u.update(u.args)
		u.args = nil
		return err
	}
	return nil
}

type GraphInfo struct {
	Print         []string
	Width, Height uint
	Ymin, Ymax    float64
}

type Grapher struct {
	m      sync.Mutex
	title  string
	vlabel string
	width  uint
	height uint
	args   []string
}

func NewGrapher() *Grapher {
	return new(Grapher)
}

func (g *Grapher) SetTitle(title string) {
	g.title = title
}

func (g *Grapher) SetVLabel(vlabel string) {
	g.vlabel = vlabel
}

func (g *Grapher) SetSize(width, height uint) {
	g.width = width
	g.height = height
}

func (g *Grapher) push(cmd string, options []string) {
	if len(options) > 0 {
		cmd += ":" + strings.Join(options, ":")
	}
	g.args = append(g.args, cmd)
}

func (g *Grapher) Def(vname, rrdfile, dsname, cf string, options ...string) {
	g.push(
		fmt.Sprintf("DEF:%s=%s:%s:%s", vname, rrdfile, dsname, cf),
		options,
	)
}

func (g *Grapher) VDef(vname, rpn string) {
	g.push("VDEF:"+vname+"="+rpn, nil)
}

func (g *Grapher) CDef(vname, rpn string) {
	g.push("CDEF:"+vname+"="+rpn, nil)
}

func (g *Grapher) Line(width float32, value, color string, options ...string) {
	line := fmt.Sprintf("LINE%f:%s", width, value)
	if color != "" {
		line += "#" + color
	}
	g.push(line, options)
}

func (g *Grapher) Print(vname, format string) {
	g.push("PRINT:"+vname+":"+format, nil)
}

func (g *Grapher) PrintT(vname, format string) {
	g.push("PRINT:"+vname+":"+format+":strftime", nil)
}
func (g *Grapher) GPrint(vname, format string) {
	g.push("GPRINT:"+vname+":"+format, nil)
}

func (g *Grapher) GPrintT(vname, format string) {
	g.push("GPRINT:"+vname+":"+format+":strftime", nil)
}

// Graph returns GraphInfo and image as []byte or error
func (g *Grapher) Graph(start, end time.Time) (GraphInfo, []byte, error) {
	return g.graph("-", start, end)
}

// SaveGraph saves image to file and returns GraphInfo or error
func (g *Grapher) SaveGraph(filename string, start, end time.Time) (GraphInfo, error) {
	gi, _, err := g.graph(filename, start, end)
	return gi, err
}
