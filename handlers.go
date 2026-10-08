package starlings

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// A handler passed to Slash, Button, or Modal asks only for what it uses and
// returns what should be sent:
//
//	func ping() string                                   { return "Pong!" }
//	func ban(a BanArgs) (string, error)                  { ... }
//	func info(i *starlings.InteractionCreate) *starlings.Embed { ... }
//	func closeTicket(p struct{ ID int64 }) starlings.Response { ... }
//
// Inputs, in this order, are each optional: the *InteractionCreate, then a
// struct whose fields are filled from the command's options, the button
// route's {parameters}, or the modal's text inputs.
//
// The result may be nothing, an error, or a value with or without an error.
// A string or *Embed is sent as a reply; a Response says how to send it. An
// empty string sends nothing, for handlers that answered on their own.
// Errors follow the rules described on UserError.
//
// The handler's shape is checked when it is registered, so a mistake stops
// the program at startup rather than when someone uses the command.

// Response is a handler result that says how it should be sent.
type Response struct {
	data   InteractionResponseData
	update bool
}

// Ephemeral returns a reply only the user can see. content is a string or an
// *Embed.
func Ephemeral(content any) Response {
	r := Reply(content)
	r.data.Flags |= MessageFlagEphemeral
	return r
}

// Reply returns a reply everyone in the channel can see. content is a string
// or an *Embed. Returning a string or *Embed directly does the same.
func Reply(content any) Response {
	return Response{data: responseData(content)}
}

// Update returns a result that edits the message holding the button or
// select, instead of sending a new one. content is a string or an *Embed.
func Update(content any) Response {
	return Response{data: responseData(content), update: true}
}

func responseData(content any) InteractionResponseData {
	data := InteractionResponseData{AllowedMentions: NoMentions()}
	switch v := content.(type) {
	case string:
		data.Content = v
	case *Embed:
		if v != nil {
			data.Embeds = []Embed{*v}
		}
	case Embed:
		data.Embeds = []Embed{v}
	case InteractionResponseData:
		return v
	default:
		panic(fmt.Sprintf("starlings: cannot send a %T; use a string or *Embed", content))
	}
	return data
}

// send delivers a handler's result.
func (r Response) send(i *InteractionCreate) error {
	if r.update {
		return i.UpdateMessage(r.data)
	}
	return i.ReplyComplex(r.data)
}

// argSource says where a handler's argument struct is filled from.
type argSource uint8

const (
	argsFromOptions argSource = iota // slash command options
	argsFromParams                   // custom-ID route parameters
	argsFromModal                    // modal text inputs
)

var (
	interactionPtrType = reflect.TypeFor[*InteractionCreate]()
	errorType          = reflect.TypeFor[error]()
	stringType         = reflect.TypeFor[string]()
	embedPtrType       = reflect.TypeFor[*Embed]()
	responseType       = reflect.TypeFor[Response]()
)

// adaptedHandler is a handler of any supported shape, ready to call.
type adaptedHandler struct {
	call    func(*InteractionCreate) error
	plan    *argPlan // set when the handler takes slash command options
	options []CommandOption
}

// adaptHandler checks handler's shape and wraps it. what names the route in
// error messages.
func adaptHandler(what string, handler any, source argSource, description string) adaptedHandler {
	switch fn := handler.(type) {
	case func(*InteractionCreate):
		return adaptedHandler{call: func(i *InteractionCreate) error { fn(i); return nil }}
	case SlashFunc:
		return adaptedHandler{call: func(i *InteractionCreate) error { fn(i); return nil }}
	case func(*InteractionCreate) error:
		return adaptedHandler{call: fn}
	case HandlerFunc:
		return adaptedHandler{call: fn}
	}

	fail := func(format string, args ...any) {
		panic("starlings: " + what + ": " + fmt.Sprintf(format, args...))
	}
	v := reflect.ValueOf(handler)
	t := v.Type()
	if v.Kind() != reflect.Func {
		fail("handler must be a function, got %T", handler)
	}

	in := 0
	wantsInteraction := t.NumIn() > in && t.In(in) == interactionPtrType
	if wantsInteraction {
		in++
	}
	var argsType reflect.Type
	if t.NumIn() > in {
		argsType = t.In(in)
		if argsType.Kind() != reflect.Struct {
			fail("parameter %d is %s; handlers take *starlings.InteractionCreate and/or a struct", in+1, argsType)
		}
		in++
	}
	if t.NumIn() != in || t.IsVariadic() {
		fail("handlers take at most *starlings.InteractionCreate and one struct")
	}

	switch t.NumOut() {
	case 0:
	case 1:
		if !t.Out(0).Implements(errorType) && !sendable(t.Out(0)) {
			fail("cannot send a %s; return a string, *Embed, Response, or error", t.Out(0))
		}
	case 2:
		if !sendable(t.Out(0)) || t.Out(1) != errorType {
			fail("two results must be (string, *Embed, or Response) and error")
		}
	default:
		fail("handlers return at most a value and an error")
	}

	out := adaptedHandler{}
	var decode func(*InteractionCreate, reflect.Value) error
	if argsType != nil {
		switch source {
		case argsFromOptions:
			plan := planArgsOf(argsType, what, description)
			out.plan = &plan
			out.options = plan.options()
			decode = plan.decode
		case argsFromParams:
			decode = planFields(argsType, what, func(i *InteractionCreate, name string) (string, bool) {
				v, ok := i.params[name]
				return v, ok
			})
		case argsFromModal:
			decode = planFields(argsType, what, func(i *InteractionCreate, name string) (string, bool) {
				if i.Data.Component(name) == nil {
					return "", false
				}
				return i.TextValue(name), true
			})
		}
	}

	out.call = func(i *InteractionCreate) error {
		args := make([]reflect.Value, 0, 2)
		if wantsInteraction {
			args = append(args, reflect.ValueOf(i))
		}
		if argsType != nil {
			value := reflect.New(argsType).Elem()
			if err := decode(i, value); err != nil {
				return err
			}
			args = append(args, value)
		}
		results := v.Call(args)
		if n := len(results); n > 0 {
			if err, _ := results[n-1].Interface().(error); err != nil {
				return err
			}
			if sendable(results[0].Type()) {
				return sendResult(i, results[0].Interface())
			}
		}
		return nil
	}
	return out
}

func sendable(t reflect.Type) bool {
	return t == stringType || t == embedPtrType || t == responseType
}

func sendResult(i *InteractionCreate, result any) error {
	switch v := result.(type) {
	case string:
		if v == "" {
			return nil
		}
		return Reply(v).send(i)
	case *Embed:
		if v == nil {
			return nil
		}
		return Reply(v).send(i)
	case Response:
		return v.send(i)
	}
	return nil
}

// planFields decodes text values into a struct by field name, for route
// parameters and modal inputs. A field's name is its `name` tag or its name
// in snake_case; fields can be strings, integers, floats, bools, or
// Snowflakes. Values come from the user's client, so a value that does not
// parse is reported to them rather than silently zeroed.
func planFields(t reflect.Type, what string, lookup func(*InteractionCreate, string) (string, bool)) func(*InteractionCreate, reflect.Value) error {
	type field struct {
		index int
		name  string
		kind  reflect.Kind
	}
	var fields []field
	for n := range t.NumField() {
		f := t.Field(n)
		if !f.IsExported() {
			continue
		}
		name := f.Tag.Get("name")
		if name == "" {
			name = snakeCase(f.Name)
		}
		switch f.Type.Kind() {
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		default:
			panic(fmt.Sprintf("starlings: %s: field %s has type %s; use text, number, bool, or Snowflake fields", what, f.Name, f.Type))
		}
		fields = append(fields, field{index: n, name: name, kind: f.Type.Kind()})
	}
	return func(i *InteractionCreate, dst reflect.Value) error {
		for _, f := range fields {
			raw, ok := lookup(i, f.name)
			if !ok {
				continue
			}
			target := dst.Field(f.index)
			if err := setText(target, f.kind, raw); err != nil {
				return UserErrorf("%q is not a valid %s.", raw, strings.ReplaceAll(f.name, "_", " "))
			}
		}
		return nil
	}
}

func setText(target reflect.Value, kind reflect.Kind, raw string) error {
	switch kind {
	case reflect.String:
		target.SetString(raw)
	case reflect.Bool:
		var b bool
		if _, err := fmt.Sscan(raw, &b); err != nil {
			return err
		}
		target.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		if _, err := fmt.Sscan(raw, &n); err != nil || target.OverflowInt(n) {
			return fmt.Errorf("invalid")
		}
		target.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var n uint64
		if _, err := fmt.Sscan(raw, &n); err != nil || target.OverflowUint(n) {
			return fmt.Errorf("invalid")
		}
		target.SetUint(n)
	case reflect.Float32, reflect.Float64:
		var f float64
		if _, err := fmt.Sscan(raw, &f); err != nil {
			return err
		}
		target.SetFloat(f)
	}
	return nil
}

// Button handles a button or select whose custom ID matches pattern, which
// may capture {parameters}. See the handler shapes above; a struct parameter
// is filled from the captured values:
//
//	bot.Button("ticket:close:{id}", func(p struct{ ID int64 }) starlings.Response {
//		...
//	})
func (c *Client) Button(pattern string, handler any) func() {
	h := adaptHandler("button "+strconv.Quote(pattern), handler, argsFromParams, "")
	return c.OnComponent(pattern, func(i *InteractionCreate) { i.c.runHandler("button "+pattern, i, h.call) })
}

// Modal handles a submitted modal whose custom ID matches pattern. A struct
// parameter is filled from the modal's text inputs, matched by custom ID.
func (c *Client) Modal(pattern string, handler any) func() {
	h := adaptHandler("modal "+strconv.Quote(pattern), handler, argsFromModal, "")
	return c.OnModal(pattern, func(i *InteractionCreate) { i.c.runHandler("modal "+pattern, i, h.call) })
}
