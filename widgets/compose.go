package widgets

import (
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/gdamore/tcell"
	"github.com/mattn/go-runewidth"

	"git.sr.ht/~sircmpwn/aerc2/config"
	"git.sr.ht/~sircmpwn/aerc2/lib/ui"
)

type headerEditor struct {
	name  string
	input *ui.TextInput
}

type Composer struct {
	headers struct {
		from    *headerEditor
		subject *headerEditor
		to      *headerEditor
	}

	config *config.AccountConfig

	editor *Terminal
	email  *os.File
	grid   *ui.Grid

	focusable []ui.DrawableInteractive
	focused   int
}

// TODO: Let caller configure headers, initial body (for replies), etc
func NewComposer(conf *config.AccountConfig) *Composer {
	grid := ui.NewGrid().Rows([]ui.GridSpec{
		{ui.SIZE_EXACT, 3},
		{ui.SIZE_WEIGHT, 1},
	}).Columns([]ui.GridSpec{
		{ui.SIZE_WEIGHT, 1},
	})

	// TODO: let user specify extra headers to edit by default
	headers := ui.NewGrid().Rows([]ui.GridSpec{
		{ui.SIZE_EXACT, 1}, // To/From
		{ui.SIZE_EXACT, 1}, // Subject
		{ui.SIZE_EXACT, 1}, // [spacer]
	}).Columns([]ui.GridSpec{
		{ui.SIZE_WEIGHT, 1},
		{ui.SIZE_WEIGHT, 1},
	})

	to := newHeaderEditor("To", "")
	from := newHeaderEditor("From", conf.From)
	subject := newHeaderEditor("Subject", "")
	headers.AddChild(to).At(0, 0)
	headers.AddChild(from).At(0, 1)
	headers.AddChild(subject).At(1, 0).Span(1, 2)
	headers.AddChild(ui.NewFill(' ')).At(2, 0).Span(1, 2)

	email, err := ioutil.TempFile("", "aerc-compose-*.eml")
	if err != nil {
		// TODO: handle this better
		return nil
	}

	// TODO: built-in config option, $EDITOR, then vi, in that order
	editor := exec.Command("vim", email.Name())
	term, _ := NewTerminal(editor)

	grid.AddChild(headers).At(0, 0)
	grid.AddChild(term).At(1, 0)

	return &Composer{
		config: conf,
		editor: term,
		email:  email,
		grid:   grid,
		// You have to backtab to get to "From", since you usually don't edit it
		focused:   1,
		focusable: []ui.DrawableInteractive{from, to, subject, term},
	}
}

func (c *Composer) Draw(ctx *ui.Context) {
	c.grid.Draw(ctx)
}

func (c *Composer) Invalidate() {
	c.grid.Invalidate()
}

func (c *Composer) OnInvalidate(fn func(d ui.Drawable)) {
	c.grid.OnInvalidate(func(_ ui.Drawable) {
		fn(c)
	})
}

func (c *Composer) Event(event tcell.Event) bool {
	return c.focusable[c.focused].Event(event)
}

func (c *Composer) Focus(focus bool) {
	c.focusable[c.focused].Focus(focus)
}

func (c *Composer) PrevField() {
	c.focusable[c.focused].Focus(false)
	c.focused--
	if c.focused == -1 {
		c.focused = len(c.focusable) - 1
	}
	c.focusable[c.focused].Focus(true)
}

func (c *Composer) NextField() {
	c.focusable[c.focused].Focus(false)
	c.focused = (c.focused + 1) % len(c.focusable)
	c.focusable[c.focused].Focus(true)
}

func newHeaderEditor(name string, value string) *headerEditor {
	return &headerEditor{
		input: ui.NewTextInput(value),
		name:  name,
	}
}

func (he *headerEditor) Draw(ctx *ui.Context) {
	name := he.name + " "
	size := runewidth.StringWidth(name)
	ctx.Fill(0, 0, size, ctx.Height(), ' ', tcell.StyleDefault)
	ctx.Printf(0, 0, tcell.StyleDefault.Bold(true), "%s", name)
	he.input.Draw(ctx.Subcontext(size, 0, ctx.Width()-size, 1))
}

func (he *headerEditor) Invalidate() {
	he.input.Invalidate()
}

func (he *headerEditor) OnInvalidate(fn func(ui.Drawable)) {
	he.input.OnInvalidate(func(_ ui.Drawable) {
		fn(he)
	})
}

func (he *headerEditor) Focus(focused bool) {
	he.input.Focus(focused)
}

func (he *headerEditor) Event(event tcell.Event) bool {
	return he.input.Event(event)
}
