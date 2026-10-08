package starlings

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// ReplyEmbed answers with one embed, visible to everyone in the channel.
func (i *InteractionCreate) ReplyEmbed(e *Embed) error {
	return i.ReplyComplex(InteractionResponseData{Embeds: []Embed{*e}, AllowedMentions: NoMentions()})
}

// ReplyEmbedEphemeral answers with one embed only the user can see.
func (i *InteractionCreate) ReplyEmbedEphemeral(e *Embed) error {
	return i.ReplyComplex(InteractionResponseData{Embeds: []Embed{*e}, Flags: MessageFlagEphemeral, AllowedMentions: NoMentions()})
}

// SendEmbed posts one embed in the channel.
func (ch *Channel) SendEmbed(e *Embed) (*Message, error) {
	return ch.SendComplex(SendData{Embeds: []Embed{*e}, AllowedMentions: NoMentions()})
}

// ReplyEmbed replies to the message with one embed.
func (msg *Message) ReplyEmbed(e *Embed) (*Message, error) {
	return msg.ReplyComplex(SendData{Embeds: []Embed{*e}, AllowedMentions: NoMentions()})
}

// Custom IDs with parameters

// CustomID joins parts into a component custom ID, such as
// CustomID("ticket", "close", ticketID) == "ticket:close:42". Values are
// escaped so a value containing ":" cannot add segments, which routes with
// {parameters} rely on. It panics if the result is longer than Discord's 100
// characters, because the component could never be sent.
func CustomID(parts ...any) string {
	segments := make([]string, len(parts))
	for n, part := range parts {
		segments[n] = escapeSegment(fmt.Sprint(part))
	}
	id := strings.Join(segments, ":")
	if len(id) > 100 {
		panic("starlings: custom ID " + strconv.Quote(id[:40]+"...") + " is longer than 100 characters")
	}
	return id
}

func escapeSegment(s string) string {
	if !strings.ContainsAny(s, ":%") {
		return s
	}
	return strings.NewReplacer("%", "%25", ":", "%3A").Replace(s)
}

// customIDPattern is a parsed route such as "ticket:close:{id}".
type customIDPattern struct {
	kind     InteractionType
	segments []string // literal, or "{name}" for a parameter
	handler  SlashFunc
	id       uint64
}

func (p customIDPattern) match(raw string) (map[string]string, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != len(p.segments) {
		return nil, false
	}
	var params map[string]string
	for n, seg := range p.segments {
		if name, ok := strings.CutPrefix(seg, "{"); ok {
			value, err := url.PathUnescape(parts[n])
			if err != nil {
				return nil, false
			}
			if params == nil {
				params = make(map[string]string, 2)
			}
			params[strings.TrimSuffix(name, "}")] = value
			continue
		}
		if parts[n] != seg {
			return nil, false
		}
	}
	return params, true
}

// onCustomIDPattern registers a route with {parameters}. Exact routes are
// checked first, then patterns in registration order.
func (c *Client) onCustomIDPattern(kind InteractionType, pattern string, fn SlashFunc) func() {
	c = c.rootClient()
	p := customIDPattern{kind: kind, segments: strings.Split(pattern, ":"), handler: fn, id: c.handlerSeq.Add(1)}
	for _, seg := range p.segments {
		if strings.HasPrefix(seg, "{") != strings.HasSuffix(seg, "}") {
			panic("starlings: malformed custom-ID route " + strconv.Quote(pattern))
		}
	}
	c.slashMu.Lock()
	c.patterns = append(c.patterns, p)
	hook := !c.slashHooked
	c.slashHooked = true
	c.slashMu.Unlock()
	if hook {
		On(c, c.routeInteraction)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			c.slashMu.Lock()
			for n, existing := range c.patterns {
				if existing.id == p.id {
					c.patterns = append(c.patterns[:n:n], c.patterns[n+1:]...)
					break
				}
			}
			c.slashMu.Unlock()
		})
	}
}

// componentRoute finds the handler for a component or modal and records
// any route parameters on the interaction.
func (c *Client) componentRoute(i *InteractionCreate) SlashFunc {
	c.slashMu.RLock()
	defer c.slashMu.RUnlock()
	if h := c.components[componentKey{i.Type, i.Data.CustomID}]; h.fn != nil {
		return h.fn
	}
	for _, p := range c.patterns {
		if p.kind != i.Type {
			continue
		}
		if params, ok := p.match(i.Data.CustomID); ok {
			i.params = params
			return p.handler
		}
	}
	return nil
}

// Param returns a parameter captured by a custom-ID route such as
// "ticket:close:{id}". Values come from the component, which any user can
// alter, so validate them like any other input.
func (i *InteractionCreate) Param(name string) string { return i.params[name] }

// ParamID returns a route parameter as a snowflake, or zero.
func (i *InteractionCreate) ParamID(name string) Snowflake {
	id, _ := ParseSnowflake(i.params[name])
	return id
}

// ParamInt returns a route parameter as an integer, or zero.
func (i *InteractionCreate) ParamInt(name string) int64 {
	n, _ := strconv.ParseInt(i.params[name], 10, 64)
	return n
}

// Pages

// Page is one page of a paged view.
type Page struct {
	Content string
	Embed   *Embed
	// Pages is the total number of pages, used to disable the arrows.
	Pages int
	// Extra holds additional components, placed below the arrows.
	Extra []Component
}

// PageFunc renders page (from zero) of a paged view. arg is the value given
// to Pager.Show, carried in the buttons' custom IDs.
type PageFunc func(i *InteractionCreate, page int, arg string) (Page, error)

// Pager is a paged view with previous and next buttons. It keeps no state:
// the page, owner, and argument travel in the buttons' custom IDs, so pages
// keep working after the bot restarts. Only the person who opened the view
// can turn its pages.
type Pager struct {
	name   string
	render PageFunc
}

// Pager registers a paged view under name, which must be unique:
//
//	warnings := bot.Pager("warnings", func(i *starlings.InteractionCreate, page int, userID string) (starlings.Page, error) {
//		...
//	})
//	return warnings.Show(i, user.ID.String(), true)
func (c *Client) Pager(name string, render PageFunc) *Pager {
	if render == nil || name == "" || strings.ContainsAny(name, ":%") {
		panic("starlings: a pager needs a name without ':' and a render function")
	}
	p := &Pager{name: name, render: render}
	c.onCustomIDPattern(InteractionMessageComponent, "sl:pg:"+name+":{owner}:{page}:{arg}", func(i *InteractionCreate) {
		i.c.runHandler("pager "+name, i, p.turn)
	})
	return p
}

// Show answers the interaction with the first page.
func (p *Pager) Show(i *InteractionCreate, arg string, ephemeral bool) error {
	data, err := p.page(i, 0, arg)
	if err != nil {
		return err
	}
	if ephemeral {
		data.Flags |= MessageFlagEphemeral
	}
	return i.ReplyComplex(data)
}

func (p *Pager) turn(i *InteractionCreate) error {
	if owner := i.ParamID("owner"); owner != i.Invoker().ID {
		return UserErrorf("Only <@%s> can turn these pages.", owner)
	}
	page := int(i.ParamInt("page"))
	data, err := p.page(i, page, i.Param("arg"))
	if err != nil {
		return err
	}
	return i.UpdateMessage(data)
}

func (p *Pager) page(i *InteractionCreate, page int, arg string) (InteractionResponseData, error) {
	page = max(page, 0)
	out, err := p.render(i, page, arg)
	if err != nil {
		return InteractionResponseData{}, err
	}
	pages := max(out.Pages, 1)
	if page >= pages {
		page = pages - 1
		if out, err = p.render(i, page, arg); err != nil {
			return InteractionResponseData{}, err
		}
	}
	owner := i.Invoker().ID
	id := func(target int) string { return CustomID("sl", "pg", p.name, owner, target, arg) }
	prev := Button(ButtonSecondary, "◀", id(page-1))
	prev.Disabled = page == 0
	label := Button(ButtonSecondary, fmt.Sprintf("%d / %d", page+1, pages), id(page)+":x")
	label.Disabled = true
	next := Button(ButtonSecondary, "▶", id(page+1))
	next.Disabled = page+1 >= pages
	data := InteractionResponseData{
		Content:         out.Content,
		AllowedMentions: NoMentions(),
		Components:      append([]Component{ActionRow(prev, label, next)}, out.Extra...),
	}
	if out.Embed != nil {
		data.Embeds = []Embed{*out.Embed}
	}
	return data, nil
}
