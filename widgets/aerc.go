package widgets

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/gdamore/tcell/v2"
	"github.com/google/shlex"
	"golang.org/x/crypto/openpgp"

	"git.sr.ht/~sircmpwn/aerc/config"
	"git.sr.ht/~sircmpwn/aerc/lib"
	"git.sr.ht/~sircmpwn/aerc/lib/ui"
	"git.sr.ht/~sircmpwn/aerc/models"
)

type Aerc struct {
	accounts    map[string]*AccountView
	cmd         func(cmd []string) error
	cmdHistory  lib.History
	complete    func(cmd string) []string
	conf        *config.AercConfig
	focused     ui.Interactive
	grid        *ui.Grid
	logger      *log.Logger
	simulating  int
	statusbar   *ui.Stack
	statusline  *StatusLine
	pendingKeys []config.KeyStroke
	prompts     *ui.Stack
	tabs        *ui.Tabs
	ui          *ui.UI
	beep        func() error
	dialog      ui.DrawableInteractive
}

type Choice struct {
	Key     string
	Text    string
	Command []string
}

func NewAerc(conf *config.AercConfig, logger *log.Logger,
	cmd func(cmd []string) error, complete func(cmd string) []string,
	cmdHistory lib.History) *Aerc {

	tabs := ui.NewTabs(&conf.Ui)

	statusbar := ui.NewStack(conf.Ui)
	statusline := NewStatusLine(conf.Ui)
	statusbar.Push(statusline)

	grid := ui.NewGrid().Rows([]ui.GridSpec{
		{ui.SIZE_EXACT, ui.Const(1)},
		{ui.SIZE_WEIGHT, ui.Const(1)},
		{ui.SIZE_EXACT, ui.Const(1)},
	}).Columns([]ui.GridSpec{
		{ui.SIZE_WEIGHT, ui.Const(1)},
	})
	grid.AddChild(tabs.TabStrip)
	grid.AddChild(tabs.TabContent).At(1, 0)
	grid.AddChild(statusbar).At(2, 0)

	aerc := &Aerc{
		accounts:   make(map[string]*AccountView),
		conf:       conf,
		cmd:        cmd,
		cmdHistory: cmdHistory,
		complete:   complete,
		grid:       grid,
		logger:     logger,
		statusbar:  statusbar,
		statusline: statusline,
		prompts:    ui.NewStack(conf.Ui),
		tabs:       tabs,
	}

	statusline.SetAerc(aerc)
	conf.Triggers.ExecuteCommand = cmd

	for i, acct := range conf.Accounts {
		view, err := NewAccountView(aerc, conf, &conf.Accounts[i], logger, aerc)
		if err != nil {
			tabs.Add(errorScreen(err.Error(), conf.Ui), acct.Name)
		} else {
			aerc.accounts[acct.Name] = view
			tabs.Add(view, acct.Name)
		}
	}

	if len(conf.Accounts) == 0 {
		wizard := NewAccountWizard(aerc.Config(), aerc)
		wizard.Focus(true)
		aerc.NewTab(wizard, "New account")
	}

	tabs.CloseTab = func(index int) {
		switch content := aerc.tabs.Tabs[index].Content.(type) {
		case *AccountView:
			return
		case *AccountWizard:
			return
		case *Composer:
			aerc.RemoveTab(content)
			content.Close()
		case *Terminal:
			content.Close(nil)
		case *MessageViewer:
			aerc.RemoveTab(content)
		}
	}

	return aerc
}

func (aerc *Aerc) OnBeep(f func() error) {
	aerc.beep = f
}

func (aerc *Aerc) Beep() {
	if aerc.beep == nil {
		aerc.logger.Printf("should beep, but no beeper")
		return
	}
	if err := aerc.beep(); err != nil {
		aerc.logger.Printf("tried to beep, but could not: %v", err)
	}
}

func (aerc *Aerc) Tick() bool {
	more := false
	for _, acct := range aerc.accounts {
		more = acct.Tick() || more
	}

	if len(aerc.prompts.Children()) > 0 {
		more = true
		previous := aerc.focused
		prompt := aerc.prompts.Pop().(*ExLine)
		prompt.finish = func() {
			aerc.statusbar.Pop()
			aerc.focus(previous)
		}

		aerc.statusbar.Push(prompt)
		aerc.focus(prompt)
	}

	return more
}

func (aerc *Aerc) Children() []ui.Drawable {
	return aerc.grid.Children()
}

func (aerc *Aerc) OnInvalidate(onInvalidate func(d ui.Drawable)) {
	aerc.grid.OnInvalidate(func(_ ui.Drawable) {
		onInvalidate(aerc)
	})
}

func (aerc *Aerc) Invalidate() {
	aerc.grid.Invalidate()
}

func (aerc *Aerc) Focus(focus bool) {
	// who cares
}

func (aerc *Aerc) Draw(ctx *ui.Context) {
	aerc.grid.Draw(ctx)
	if aerc.dialog != nil {
		aerc.dialog.Draw(ctx.Subcontext(4, ctx.Height()/2-2,
			ctx.Width()-8, 4))
	}
}

func (aerc *Aerc) getBindings() *config.KeyBindings {
	switch view := aerc.SelectedTab().(type) {
	case *AccountView:
		return aerc.conf.Bindings.MessageList
	case *AccountWizard:
		return aerc.conf.Bindings.AccountWizard
	case *Composer:
		switch view.Bindings() {
		case "compose::editor":
			return aerc.conf.Bindings.ComposeEditor
		case "compose::review":
			return aerc.conf.Bindings.ComposeReview
		default:
			return aerc.conf.Bindings.Compose
		}
	case *MessageViewer:
		return aerc.conf.Bindings.MessageView
	case *Terminal:
		return aerc.conf.Bindings.Terminal
	default:
		return aerc.conf.Bindings.Global
	}
}

func (aerc *Aerc) simulate(strokes []config.KeyStroke) {
	aerc.pendingKeys = []config.KeyStroke{}
	aerc.simulating += 1
	for _, stroke := range strokes {
		simulated := tcell.NewEventKey(
			stroke.Key, stroke.Rune, tcell.ModNone)
		aerc.Event(simulated)
	}
	aerc.simulating -= 1
}

func (aerc *Aerc) Event(event tcell.Event) bool {
	if aerc.dialog != nil {
		return aerc.dialog.Event(event)
	}

	if aerc.focused != nil {
		return aerc.focused.Event(event)
	}

	switch event := event.(type) {
	case *tcell.EventKey:
		aerc.statusline.Expire()
		aerc.pendingKeys = append(aerc.pendingKeys, config.KeyStroke{
			Key:  event.Key(),
			Rune: event.Rune(),
		})
		aerc.statusline.Invalidate()
		bindings := aerc.getBindings()
		incomplete := false
		result, strokes := bindings.GetBinding(aerc.pendingKeys)
		switch result {
		case config.BINDING_FOUND:
			aerc.simulate(strokes)
			return true
		case config.BINDING_INCOMPLETE:
			incomplete = true
		case config.BINDING_NOT_FOUND:
		}
		if bindings.Globals {
			result, strokes = aerc.conf.Bindings.Global.
				GetBinding(aerc.pendingKeys)
			switch result {
			case config.BINDING_FOUND:
				aerc.simulate(strokes)
				return true
			case config.BINDING_INCOMPLETE:
				incomplete = true
			case config.BINDING_NOT_FOUND:
			}
		}
		if !incomplete {
			aerc.pendingKeys = []config.KeyStroke{}
			exKey := bindings.ExKey
			if aerc.simulating > 0 {
				// Keybindings still use : even if you change the ex key
				exKey = aerc.conf.Bindings.Global.ExKey
			}
			if event.Key() == exKey.Key && event.Rune() == exKey.Rune {
				aerc.BeginExCommand("")
				return true
			}
			interactive, ok := aerc.tabs.Tabs[aerc.tabs.Selected].Content.(ui.Interactive)
			if ok {
				return interactive.Event(event)
			}
			return false
		}
	case *tcell.EventMouse:
		if event.Buttons() == tcell.ButtonNone {
			return false
		}
		x, y := event.Position()
		aerc.grid.MouseEvent(x, y, event)
		return true
	}
	return false
}

func (aerc *Aerc) Config() *config.AercConfig {
	return aerc.conf
}

func (aerc *Aerc) Logger() *log.Logger {
	return aerc.logger
}

func (aerc *Aerc) SelectedAccount() *AccountView {
	switch tab := aerc.SelectedTab().(type) {
	case *AccountView:
		return tab
	case *MessageViewer:
		return tab.SelectedAccount()
	case *Composer:
		return tab.Account()
	}
	return nil
}

func (aerc *Aerc) SelectedTab() ui.Drawable {
	return aerc.tabs.Tabs[aerc.tabs.Selected].Content
}

func (aerc *Aerc) SelectedTabIndex() int {
	return aerc.tabs.Selected
}

func (aerc *Aerc) NumTabs() int {
	return len(aerc.tabs.Tabs)
}

func (aerc *Aerc) NewTab(clickable ui.Drawable, name string) *ui.Tab {
	tab := aerc.tabs.Add(clickable, name)
	aerc.tabs.Select(len(aerc.tabs.Tabs) - 1)
	return tab
}

func (aerc *Aerc) RemoveTab(tab ui.Drawable) {
	aerc.tabs.Remove(tab)
}

func (aerc *Aerc) ReplaceTab(tabSrc ui.Drawable, tabTarget ui.Drawable, name string) {
	aerc.tabs.Replace(tabSrc, tabTarget, name)
}

func (aerc *Aerc) MoveTab(i int) {
	aerc.tabs.MoveTab(i)
}

func (aerc *Aerc) PinTab() {
	aerc.tabs.PinTab()
}

func (aerc *Aerc) UnpinTab() {
	aerc.tabs.UnpinTab()
}

func (aerc *Aerc) NextTab() {
	aerc.tabs.NextTab()
}

func (aerc *Aerc) PrevTab() {
	aerc.tabs.PrevTab()
}

func (aerc *Aerc) SelectTab(name string) bool {
	for i, tab := range aerc.tabs.Tabs {
		if tab.Name == name {
			aerc.tabs.Select(i)
			return true
		}
	}
	return false
}

func (aerc *Aerc) SelectTabIndex(index int) bool {
	for i := range aerc.tabs.Tabs {
		if i == index {
			aerc.tabs.Select(i)
			return true
		}
	}
	return false
}

func (aerc *Aerc) TabNames() []string {
	names := make([]string, len(aerc.tabs.Tabs))
	for i, tab := range aerc.tabs.Tabs {
		names[i] = tab.Name
	}
	return names
}

func (aerc *Aerc) SelectPreviousTab() bool {
	return aerc.tabs.SelectPrevious()
}

// TODO: Use per-account status lines, but a global ex line
func (aerc *Aerc) SetStatus(status string) *StatusMessage {
	return aerc.statusline.Set(status)
}

func (aerc *Aerc) SetError(status string) *StatusMessage {
	return aerc.statusline.SetError(status)
}

func (aerc *Aerc) PushStatus(text string, expiry time.Duration) *StatusMessage {
	return aerc.statusline.Push(text, expiry)
}

func (aerc *Aerc) PushError(text string) *StatusMessage {
	return aerc.statusline.PushError(text)
}

func (aerc *Aerc) PushSuccess(text string) *StatusMessage {
	return aerc.statusline.PushSuccess(text)
}

func (aerc *Aerc) focus(item ui.Interactive) {
	if aerc.focused == item {
		return
	}
	if aerc.focused != nil {
		aerc.focused.Focus(false)
	}
	aerc.focused = item
	interactive, ok := aerc.tabs.Tabs[aerc.tabs.Selected].Content.(ui.Interactive)
	if item != nil {
		item.Focus(true)
		if ok {
			interactive.Focus(false)
		}
	} else {
		if ok {
			interactive.Focus(true)
		}
	}
}

func (aerc *Aerc) BeginExCommand(cmd string) {
	previous := aerc.focused
	exline := NewExLine(aerc.conf, cmd, func(cmd string) {
		parts, err := shlex.Split(cmd)
		if err != nil {
			aerc.PushError(err.Error())
		}
		err = aerc.cmd(parts)
		if err != nil {
			aerc.PushError(err.Error())
		}
		// only add to history if this is an unsimulated command,
		// ie one not executed from a keybinding
		if aerc.simulating == 0 {
			aerc.cmdHistory.Add(cmd)
		}
	}, func() {
		aerc.statusbar.Pop()
		aerc.focus(previous)
	}, func(cmd string) []string {
		return aerc.complete(cmd)
	}, aerc.cmdHistory)
	aerc.statusbar.Push(exline)
	aerc.focus(exline)
}

func (aerc *Aerc) RegisterPrompt(prompt string, cmd []string) {
	p := NewPrompt(aerc.conf, prompt, func(text string) {
		if text != "" {
			cmd = append(cmd, text)
		}
		err := aerc.cmd(cmd)
		if err != nil {
			aerc.PushError(err.Error())
		}
	}, func(cmd string) []string {
		return nil // TODO: completions
	})
	aerc.prompts.Push(p)
}

func (aerc *Aerc) RegisterChoices(choices []Choice) {
	cmds := make(map[string][]string)
	texts := []string{}
	for _, c := range choices {
		text := fmt.Sprintf("[%s] %s", c.Key, c.Text)
		if strings.Contains(c.Text, c.Key) {
			text = strings.Replace(c.Text, c.Key, "["+c.Key+"]", 1)
		}
		texts = append(texts, text)
		cmds[c.Key] = c.Command
	}
	prompt := strings.Join(texts, ", ") + "? "
	p := NewPrompt(aerc.conf, prompt, func(text string) {
		cmd, ok := cmds[text]
		if !ok {
			return
		}
		err := aerc.cmd(cmd)
		if err != nil {
			aerc.PushError(err.Error())
		}
	}, func(cmd string) []string {
		return nil // TODO: completions
	})
	aerc.prompts.Push(p)
}

func (aerc *Aerc) Mailto(addr *url.URL) error {
	acct := aerc.SelectedAccount()
	if acct == nil {
		return errors.New("No account selected")
	}

	var subject string
	h := &mail.Header{}
	to, err := mail.ParseAddressList(addr.Opaque)
	if err != nil {
		return fmt.Errorf("Could not parse to: %w", err)
	}
	h.SetAddressList("to", to)
	for key, vals := range addr.Query() {
		switch strings.ToLower(key) {
		case "cc":
			list, err := mail.ParseAddressList(strings.Join(vals, ","))
			if err != nil {
				break
			}
			h.SetAddressList("Cc", list)
		case "in-reply-to":
			for i, msgID := range vals {
				if len(msgID) > 1 && msgID[0] == '<' &&
					msgID[len(msgID)-1] == '>' {
					vals[i] = msgID[1 : len(msgID)-1]
				}
			}
			h.SetMsgIDList("In-Reply-To", vals)
		case "subject":
			subject = strings.Join(vals, ",")
			h.SetText("Subject", subject)
		default:
			// any other header gets ignored on purpose to avoid control headers
			// being injected
		}
	}

	composer, err := NewComposer(aerc, acct, aerc.Config(),
		acct.AccountConfig(), acct.Worker(), "", h, models.OriginalMail{})
	if err != nil {
		return nil
	}
	composer.FocusSubject()
	title := "New email"
	if subject != "" {
		title = subject
		composer.FocusTerminal()
	}
	tab := aerc.NewTab(composer, title)
	composer.OnHeaderChange("Subject", func(subject string) {
		if subject == "" {
			tab.Name = "New email"
		} else {
			tab.Name = subject
		}
		tab.Content.Invalidate()
	})
	return nil
}

func (aerc *Aerc) CloseBackends() error {
	var returnErr error
	for _, acct := range aerc.accounts {
		var raw interface{} = acct.worker.Backend
		c, ok := raw.(io.Closer)
		if !ok {
			continue
		}
		err := c.Close()
		if err != nil {
			returnErr = err
			aerc.logger.Printf("Closing backend failed for %v: %v\n",
				acct.Name(), err)
		}
	}
	return returnErr
}

func (aerc *Aerc) AddDialog(d ui.DrawableInteractive) {
	aerc.dialog = d
	aerc.dialog.OnInvalidate(func(_ ui.Drawable) {
		aerc.Invalidate()
	})
	aerc.Invalidate()
}

func (aerc *Aerc) CloseDialog() {
	aerc.dialog = nil
	aerc.Invalidate()
}

func (aerc *Aerc) GetPassword(title string, prompt string) (chText chan string, chErr chan error) {
	chText = make(chan string, 1)
	chErr = make(chan error, 1)
	getPasswd := NewGetPasswd(title, prompt, aerc.conf, func(pw string, err error) {
		defer func() {
			close(chErr)
			close(chText)
			aerc.CloseDialog()
		}()
		if err != nil {
			chErr <- err
			return
		}
		chErr <- nil
		chText <- pw
	})
	aerc.AddDialog(getPasswd)

	return
}

func (aerc *Aerc) Initialize(ui *ui.UI) {
	aerc.ui = ui
}

func (aerc *Aerc) DecryptKeys(keys []openpgp.Key, symmetric bool) (b []byte, err error) {
	for _, key := range keys {
		ident := key.Entity.PrimaryIdentity()
		chPass, chErr := aerc.GetPassword("Decrypt PGP private key",
			fmt.Sprintf("Enter password for %s (%8X)\nPress <ESC> to cancel",
				ident.Name, key.PublicKey.KeyId))

		for {
			select {
			case err = <-chErr:
				if err != nil {
					return nil, err
				}
				pass := <-chPass
				err = key.PrivateKey.Decrypt([]byte(pass))
				return nil, err
			default:
				aerc.ui.Tick()
			}
		}
	}
	return nil, err
}

// errorScreen is a widget that draws an error in the middle of the context
func errorScreen(s string, conf config.UIConfig) ui.Drawable {
	errstyle := conf.GetStyle(config.STYLE_ERROR)
	text := ui.NewText(s, errstyle).Strategy(ui.TEXT_CENTER)
	grid := ui.NewGrid().Rows([]ui.GridSpec{
		{ui.SIZE_WEIGHT, ui.Const(1)},
		{ui.SIZE_EXACT, ui.Const(1)},
		{ui.SIZE_WEIGHT, ui.Const(1)},
	}).Columns([]ui.GridSpec{
		{ui.SIZE_WEIGHT, ui.Const(1)},
	})
	grid.AddChild(ui.NewFill(' ')).At(0, 0)
	grid.AddChild(text).At(1, 0)
	grid.AddChild(ui.NewFill(' ')).At(2, 0)
	return grid
}
