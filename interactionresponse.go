package starlings

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
)

// CallbackType is how an interaction is answered.
type CallbackType int

const (
	CallbackPong                     CallbackType = 1
	CallbackChannelMessageWithSource CallbackType = 4 // reply now
	CallbackDeferredChannelMessage   CallbackType = 5 // "thinking...", reply later
	CallbackDeferredUpdateMessage    CallbackType = 6 // component: ack, edit later
	CallbackUpdateMessage            CallbackType = 7 // component: edit the message now
	CallbackAutocompleteResult       CallbackType = 8
	CallbackModal                    CallbackType = 9
	CallbackLaunchActivity           CallbackType = 12
)

// InteractionResponse is the body of an interaction callback.
type InteractionResponse struct {
	Type CallbackType             `json:"type"`
	Data *InteractionResponseData `json:"data,omitzero"`
}

// InteractionResponseData carries the message, autocomplete choices or modal
// that answers an interaction. Which fields matter depends on the callback
// type.
type InteractionResponseData struct {
	Content         string           `json:"content,omitzero"`
	TTS             bool             `json:"tts,omitzero"`
	Embeds          []Embed          `json:"embeds,omitzero"`
	Components      []Component      `json:"components,omitzero"`
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitzero"`
	Flags           MessageFlags     `json:"flags,omitzero"`
	Poll            *Poll            `json:"poll,omitzero"`
	Attachments     []Attachment     `json:"attachments,omitzero"`

	// Autocomplete only. Max 25.
	Choices []CommandChoice `json:"choices,omitzero"`

	// Modal only.
	CustomID string `json:"custom_id,omitzero"`
	Title    string `json:"title,omitzero"`
}

// ErrInteractionAlreadyAnswered is returned when a second initial response is
// attempted. Discord accepts exactly one callback per interaction; everything
// after it must be a follow-up.
var ErrInteractionAlreadyAnswered = errors.New(
	"starlings: this interaction has already been answered - use Followup instead")

// route is the rate-limit key for this interaction's callback and webhook
// requests. Discord limits them per interaction, so the key includes the
// interaction ID; the token stays out of it because route keys can be logged.
func (i *InteractionCreate) route(method, suffix string) string {
	return method + " /interactions/" + i.ID.String() + suffix
}

// claimAnswer records the initial callback type. Only the first caller wins.
func (i *InteractionCreate) claimAnswer(t CallbackType) bool {
	return atomic.CompareAndSwapInt32(&i.answered, 0, int32(max(t, 1)))
}

// Answered reports whether the initial response has been sent or is being
// sent. After that, use EditResponse or Followup.
func (i *InteractionCreate) Answered() bool { return atomic.LoadInt32(&i.answered) != 0 }

// Deferred reports whether the initial response was Defer or DeferUpdate, so
// the visible result still has to be sent with EditResponse or Followup.
func (i *InteractionCreate) Deferred() bool {
	t := CallbackType(atomic.LoadInt32(&i.answered))
	return t == CallbackDeferredChannelMessage || t == CallbackDeferredUpdateMessage
}

// Respond sends the initial answer to an interaction.
//
// Discord gives you **three seconds**. If the work takes longer, call Defer
// first, which shows "thinking..." and buys fifteen minutes for Followup.
func (i *InteractionCreate) Respond(ctx context.Context, resp InteractionResponse) error {
	if !i.claimAnswer(resp.Type) {
		return ErrInteractionAlreadyAnswered
	}
	if i.respondHTTP != nil {
		return i.respondHTTP(ctx, resp, nil)
	}
	return i.c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/interactions/" + i.ID.String() + "/" + i.Token + "/callback",
		Route:  i.route("POST", "/callback"),
		Body:   resp,
	}, nil)
}

// RespondFiles sends the initial interaction response with file attachments.
func (i *InteractionCreate) RespondFiles(ctx context.Context, resp InteractionResponse, files ...File) error {
	if !i.claimAnswer(resp.Type) {
		return ErrInteractionAlreadyAnswered
	}
	if i.respondHTTP != nil {
		return i.respondHTTP(ctx, resp, files)
	}
	return i.c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/interactions/" + i.ID.String() + "/" + i.Token + "/callback",
		Route:  i.route("POST", "/callback"),
		Body:   resp,
		Files:  files,
	}, nil)
}

// ReplyFiles answers an interaction with a message and attachments.
func (i *InteractionCreate) ReplyFiles(data InteractionResponseData, files ...File) error {
	return i.RespondFiles(context.Background(), InteractionResponse{
		Type: CallbackChannelMessageWithSource,
		Data: &data,
	}, files...)
}

// Reply answers with a message, visible to everyone in the channel.
func (i *InteractionCreate) Reply(content string) error {
	return i.ReplyContext(context.Background(), content)
}

// ReplyContext is Reply with an application-owned deadline or cancellation.
func (i *InteractionCreate) ReplyContext(ctx context.Context, content string) error {
	return i.Respond(ctx, InteractionResponse{
		Type: CallbackChannelMessageWithSource,
		Data: &InteractionResponseData{Content: content},
	})
}

// ReplyEphemeral answers with a message only the invoking user can see, which
// is the polite default for errors and for anything noisy.
func (i *InteractionCreate) ReplyEphemeral(content string) error {
	return i.ReplyEphemeralContext(context.Background(), content)
}

// ReplyEphemeralContext sends a private reply using the supplied context.
func (i *InteractionCreate) ReplyEphemeralContext(ctx context.Context, content string) error {
	return i.Respond(ctx, InteractionResponse{
		Type: CallbackChannelMessageWithSource,
		Data: &InteractionResponseData{Content: content, Flags: MessageFlagEphemeral},
	})
}

// ReplyComplex answers with embeds, components or flags.
func (i *InteractionCreate) ReplyComplex(data InteractionResponseData) error {
	return i.Respond(context.Background(), InteractionResponse{
		Type: CallbackChannelMessageWithSource,
		Data: &data,
	})
}

// Defer acknowledges the interaction and shows "thinking...", giving you
// fifteen minutes to send the real answer with Followup.
//
// Call this immediately if the handler does anything slow - an HTTP request, a
// database query - because missing the three-second deadline shows the user a
// permanent "application did not respond".
func (i *InteractionCreate) Defer(ephemeral bool) error {
	var data *InteractionResponseData
	if ephemeral {
		data = &InteractionResponseData{Flags: MessageFlagEphemeral}
	}
	return i.Respond(context.Background(), InteractionResponse{
		Type: CallbackDeferredChannelMessage,
		Data: data,
	})
}

// Followup sends an additional message after the initial response. It is how
// you answer once Defer has bought you time.
func (i *InteractionCreate) Followup(ctx context.Context, data InteractionResponseData) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token,
		Route:  i.route("POST", ""),
		Body:   data,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// FollowupFiles sends an interaction follow-up with attachments.
func (i *InteractionCreate) FollowupFiles(ctx context.Context, data InteractionResponseData, files ...File) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token,
		Route:  i.route("POST", ""),
		Body:   data,
		Files:  files,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// Response fetches the initial interaction response.
func (i *InteractionCreate) Response(ctx context.Context) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token + "/messages/@original",
		Route:  i.route("GET", "/messages/@original"),
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// FollowupMessage fetches one follow-up response.
func (i *InteractionCreate) FollowupMessage(ctx context.Context, messageID Snowflake) (*Message, error) {
	return i.c.WebhookMessageByID(ctx, i.ApplicationID, i.Token, messageID)
}

// EditFollowup rewrites one follow-up response.
func (i *InteractionCreate) EditFollowup(ctx context.Context, messageID Snowflake, data InteractionResponseData) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token + "/messages/" + messageID.String(),
		Route:  i.route("PATCH", "/messages/{id}"),
		Body:   data,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditFollowupFiles rewrites one follow-up response and uploads files.
func (i *InteractionCreate) EditFollowupFiles(ctx context.Context, messageID Snowflake, data InteractionResponseData, files ...File) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token + "/messages/" + messageID.String(),
		Route:  i.route("PATCH", "/messages/{id}"),
		Body:   data,
		Files:  files,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// DeleteFollowup removes one follow-up response.
func (i *InteractionCreate) DeleteFollowup(ctx context.Context, messageID Snowflake) error {
	return i.c.DeleteWebhookMessage(ctx, i.ApplicationID, i.Token, messageID)
}

// EditResponse rewrites the initial response, which is the other way to finish
// a deferred interaction.
func (i *InteractionCreate) EditResponse(ctx context.Context, data InteractionResponseData) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token + "/messages/@original",
		Route:  i.route("PATCH", "/messages/@original"),
		Body:   data,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditResponseFiles edits the original interaction response and uploads files.
func (i *InteractionCreate) EditResponseFiles(ctx context.Context, data InteractionResponseData, files ...File) (*Message, error) {
	var msg Message
	err := i.c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token + "/messages/@original",
		Route:  i.route("PATCH", "/messages/@original"),
		Body:   data,
		Files:  files,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// DeleteResponse removes the initial response.
func (i *InteractionCreate) DeleteResponse(ctx context.Context) error {
	return i.c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/webhooks/" + i.ApplicationID.String() + "/" + i.Token + "/messages/@original",
		Route:  i.route("DELETE", "/messages/@original"),
	}, nil)
}

// UpdateMessage edits the message a button or select menu belongs to, instead
// of posting a new one. Only valid for component interactions.
func (i *InteractionCreate) UpdateMessage(data InteractionResponseData) error {
	return i.Respond(context.Background(), InteractionResponse{
		Type: CallbackUpdateMessage,
		Data: &data,
	})
}

// UpdateMessageFiles edits a component's message and uploads attachments.
func (i *InteractionCreate) UpdateMessageFiles(data InteractionResponseData, files ...File) error {
	return i.RespondFiles(context.Background(), InteractionResponse{
		Type: CallbackUpdateMessage,
		Data: &data,
	}, files...)
}

// Autocomplete answers an autocomplete interaction with up to 25 suggestions.
func (i *InteractionCreate) Autocomplete(choices ...CommandChoice) error {
	return i.Respond(context.Background(), InteractionResponse{
		Type: CallbackAutocompleteResult,
		Data: &InteractionResponseData{Choices: choices},
	})
}

// Modal opens a form. The custom ID comes back on the modal-submit
// interaction, which is how you match a submission to what opened it.
func (i *InteractionCreate) Modal(customID, title string, components ...Component) error {
	return i.Respond(context.Background(), InteractionResponse{
		Type: CallbackModal,
		Data: &InteractionResponseData{
			CustomID:   customID,
			Title:      title,
			Components: components,
		},
	})
}

// Option access

// Option returns the named command option, or nil. Subcommands are searched
// too, so a handler need not care whether the command is nested.
func (d *InteractionData) Option(name string) *InteractionOption {
	return findOption(d.Options, name)
}

// String returns a named string option, or "" when it is absent. It is the
// short form of i.Data.Option(name).String().
func (i *InteractionCreate) String(name string) string { return i.Data.Option(name).String() }

// Int returns a named integer option, or 0 when it is absent.
func (i *InteractionCreate) Int(name string) int64 { return i.Data.Option(name).Int() }

// Float returns a named number option, or 0 when it is absent.
func (i *InteractionCreate) Float(name string) float64 { return i.Data.Option(name).Float() }

// Bool returns a named boolean option, or false when it is absent.
func (i *InteractionCreate) Bool(name string) bool { return i.Data.Option(name).Bool() }

// OptionID returns a named snowflake option, or zero when it is absent.
func (i *InteractionCreate) OptionID(name string) Snowflake { return i.Data.Option(name).Snowflake() }

func findOption(opts []InteractionOption, name string) *InteractionOption {
	for i := range opts {
		if opts[i].Name == name {
			return &opts[i]
		}
		if found := findOption(opts[i].Options, name); found != nil {
			return found
		}
	}
	return nil
}

// String returns the option's text value, or "" if it is absent or a
// different type.
func (o *InteractionOption) String() string {
	if o == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(o.Value, &s); err != nil {
		return ""
	}
	return s
}

// Int returns the option's integer value, or 0.
func (o *InteractionOption) Int() int64 {
	if o == nil {
		return 0
	}
	var n int64
	if err := json.Unmarshal(o.Value, &n); err != nil {
		return 0
	}
	return n
}

// Float returns the option's number value, or 0.
func (o *InteractionOption) Float() float64 {
	if o == nil {
		return 0
	}
	var f float64
	if err := json.Unmarshal(o.Value, &f); err != nil {
		return 0
	}
	return f
}

// Bool returns the option's boolean value, or false.
func (o *InteractionOption) Bool() bool {
	if o == nil {
		return false
	}
	var b bool
	if err := json.Unmarshal(o.Value, &b); err != nil {
		return false
	}
	return b
}

// Snowflake returns the option's ID value for user, channel, role and
// mentionable options, or zero.
func (o *InteractionOption) Snowflake() Snowflake {
	if o == nil {
		return 0
	}
	var s string
	if err := json.Unmarshal(o.Value, &s); err != nil {
		return 0
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return Snowflake(n)
}

// UserOption resolves a user option to the full user object, which Discord
// sends alongside the interaction so no extra request is needed.
func (i *InteractionCreate) UserOption(name string) *User {
	opt := i.Data.Option(name)
	if opt == nil || i.Data.Resolved == nil {
		return nil
	}
	return i.Data.Resolved.Users[opt.Snowflake()]
}

// ChannelOption resolves a channel option.
func (i *InteractionCreate) ChannelOption(name string) *Channel {
	opt := i.Data.Option(name)
	if opt == nil || i.Data.Resolved == nil {
		return nil
	}
	return i.Data.Resolved.Channels[opt.Snowflake()]
}

// RoleOption resolves a role option.
func (i *InteractionCreate) RoleOption(name string) *Role {
	opt := i.Data.Option(name)
	if opt == nil || i.Data.Resolved == nil {
		return nil
	}
	return i.Data.Resolved.Roles[opt.Snowflake()]
}
