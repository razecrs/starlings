package starlings

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEmbedHelpersStayWithinDiscordLimits(t *testing.T) {
	e := NewEmbed(strings.Repeat("t", 300)).
		SetDescription(strings.Repeat("d", 5000)).
		SetFooter(strings.Repeat("f", 3000), "")
	for range 30 {
		e.AddField("", strings.Repeat("v", 2000), false)
	}
	if utf8.RuneCountInString(e.Title) != 256 || !strings.HasSuffix(e.Title, "…") {
		t.Fatalf("title has %d characters", utf8.RuneCountInString(e.Title))
	}
	if utf8.RuneCountInString(e.Description) != 4096 || utf8.RuneCountInString(e.Footer.Text) != 2048 {
		t.Fatal("description or footer was not shortened")
	}
	if len(e.Fields) != 25 || e.Fields[0].Name != "​" || utf8.RuneCountInString(e.Fields[0].Value) != 1024 {
		t.Fatalf("fields = %d, first name %q", len(e.Fields), e.Fields[0].Name)
	}
}

func TestCustomIDEscapesValues(t *testing.T) {
	id := CustomID("ticket", "close", "a:b%c")
	if id != "ticket:close:a%3Ab%25c" {
		t.Fatalf("CustomID = %q", id)
	}
	p := customIDPattern{segments: strings.Split("ticket:close:{reason}", ":")}
	params, ok := p.match(id)
	if !ok || params["reason"] != "a:b%c" {
		t.Fatalf("match = %v, %v", params, ok)
	}
	if _, ok := p.match("ticket:close:a:b"); ok {
		t.Fatal("an unescaped ':' must not match a one-segment parameter")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a custom ID over 100 characters was accepted")
		}
	}()
	CustomID(strings.Repeat("x", 101))
}

func componentInteraction(c *Client, customID string, user Snowflake) *InteractionCreate {
	i := &InteractionCreate{Interaction: Interaction{
		ID: freshSnowflake(), ApplicationID: 2, Token: "t", Type: InteractionMessageComponent, GuildID: 1,
		Data:   InteractionData{CustomID: customID},
		Member: &Member{User: &User{ID: user}},
	}}
	i.bind(c)
	return i
}

func TestParameterRoutesCaptureValues(t *testing.T) {
	c, _ := autoDeferClient(t, 0)
	got := make(chan Snowflake, 1)
	c.OnComponent("ticket:close:{id}", func(i *InteractionCreate) {
		got <- i.ParamID("id")
		_ = i.DeferUpdate()
	})
	c.routeInteraction(componentInteraction(c, CustomID("ticket", "close", Snowflake(42)), 9))
	if id := <-got; id != 42 {
		t.Fatalf("id = %d", id)
	}
}

func TestPagerTurnsPagesForItsOwnerOnly(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	pages := c.Pager("items", func(_ *InteractionCreate, page int, arg string) (Page, error) {
		return Page{Content: arg + " page " + itoa(page+1), Pages: 3}, nil
	})
	open := commandInteraction(c, "items")
	open.Member = &Member{User: &User{ID: 9}}
	if err := pages.Show(open, "fruit", false); err != nil {
		t.Fatal(err)
	}
	if body := log.sentBodies(); !strings.Contains(body, "fruit page 1") || !strings.Contains(body, `"custom_id":"sl:pg:items:9:1:fruit"`) {
		t.Fatalf("first page = %s", body)
	}

	c.routeInteraction(componentInteraction(c, "sl:pg:items:9:2:fruit", 9))
	if body := log.sentBodies(); !strings.Contains(body, "fruit page 3") {
		t.Fatalf("owner could not turn to page 3: %s", body)
	}

	c.routeInteraction(componentInteraction(c, "sl:pg:items:9:1:fruit", 10))
	calls := log.get()
	if last := calls[len(calls)-1]; last != "POST /interactions/T/callback type=4 flags=64" {
		t.Fatalf("someone else turning the page = %s, want a private refusal", last)
	}
}
