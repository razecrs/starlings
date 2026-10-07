package starlings

import (
	"context"
	"sync"
)

type componentKey struct {
	kind InteractionType
	id   string
}

type componentHandler struct {
	id uint64
	fn SlashFunc
}

// OnComponent handles a button or select with exactly this custom ID. It
// returns an unsubscribe function. Check the invoking user before handling
// controls intended for one person; custom IDs are routing data, not authority.
func (c *Client) OnComponent(customID string, fn SlashFunc) func() {
	return c.onCustomID(InteractionMessageComponent, customID, fn)
}

// OnModal handles submissions of the modal with exactly this custom ID.
// Use i.TextValue to read a text input without walking component layouts.
func (c *Client) OnModal(customID string, fn SlashFunc) func() {
	return c.onCustomID(InteractionModalSubmit, customID, fn)
}

func (c *Client) onCustomID(kind InteractionType, customID string, fn SlashFunc) func() {
	c = c.rootClient()
	if customID == "" || fn == nil {
		panic("starlings: custom-ID routes need a nonempty ID and handler")
	}
	key := componentKey{kind, customID}
	id := c.handlerSeq.Add(1)
	c.slashMu.Lock()
	if c.components == nil {
		c.components = make(map[componentKey]componentHandler)
	}
	c.components[key] = componentHandler{id, fn}
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
			if c.components[key].id == id {
				delete(c.components, key)
			}
			c.slashMu.Unlock()
		})
	}
}

// TextValue reads a submitted modal text input, including inputs inside
// labels. Missing inputs and non-text components return an empty string.
func (i *InteractionCreate) TextValue(customID string) string {
	component := i.Data.Component(customID)
	if component == nil {
		return ""
	}
	value, _ := component.Value.(string)
	return value
}

// DeferUpdate acknowledges a component without displaying a new thinking
// message. EditResponse can update the component's original message later.
func (i *InteractionCreate) DeferUpdate() error {
	return i.Respond(context.Background(), InteractionResponse{Type: CallbackDeferredUpdateMessage})
}

// MemberOption resolves a selected member and joins its user data. Discord
// sends these separately. No REST request or cache is needed.
func (i *InteractionCreate) MemberOption(name string) *Member {
	if i.Data.Resolved == nil {
		return nil
	}
	id := i.OptionID(name)
	member := i.Data.Resolved.Members[id]
	if member == nil {
		return nil
	}
	copy := cloneMember(*member)
	if user := i.Data.Resolved.Users[id]; user != nil {
		copy.User = Ref(*user)
	}
	return &copy
}

// AttachmentOption resolves a selected attachment without another request.
func (i *InteractionCreate) AttachmentOption(name string) *Attachment {
	if i.Data.Resolved == nil {
		return nil
	}
	value := i.Data.Resolved.Attachments[i.OptionID(name)]
	if value == nil {
		return nil
	}
	return Ref(*value)
}
