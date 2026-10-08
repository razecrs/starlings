package starlings

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type argKind uint8

const (
	argString argKind = iota + 1
	argInt
	argUint
	argFloat
	argBool
	argDuration
	argSnowflake
	argUser
	argMember
	argRole
	argChannel
	argAttachment
)

type argField struct {
	index    int
	kind     argKind
	option   CommandOption
	optional bool
}

type argPlan struct {
	fields []argField
}

var (
	commandNamePattern = regexp.MustCompile(`^[-_\p{L}\p{N}\p{Devanagari}\p{Thai}]{1,32}$`)
	snowflakeType      = reflect.TypeFor[Snowflake]()
	durationType       = reflect.TypeFor[time.Duration]()
)

func planArgsOf(t reflect.Type, command, description string) argPlan {
	fail := func(format string, args ...any) {
		panic(fmt.Sprintf("starlings: %s: ", command) + fmt.Sprintf(format, args...))
	}
	checkName(strings.TrimPrefix(command, "/"), fail)
	if n := utf8.RuneCountInString(description); n < 1 || n > 100 {
		fail("description must be 1-100 characters, got %d", n)
	}

	if t.Kind() != reflect.Struct {
		fail("arguments must be a struct, got %s", t)
	}
	var plan argPlan
	seen := map[string]bool{}
	for index := range t.NumField() {
		field := t.Field(index)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("name")
		if name == "" {
			name = snakeCase(field.Name)
		}
		checkName(name, fail)
		if seen[name] {
			fail("two options are named %q", name)
		}
		seen[name] = true

		kind, optionType := argKindOf(field.Type)
		if kind == 0 {
			fail("field %s has type %s, which cannot be a command option", field.Name, field.Type)
		}
		desc := field.Tag.Get("desc")
		if desc == "" {
			desc = name
		}
		if n := utf8.RuneCountInString(desc); n > 100 {
			fail("description of %q is %d characters; Discord allows 100", name, n)
		}
		_, optional := field.Tag.Lookup("optional")
		_, autocomplete := field.Tag.Lookup("autocomplete")
		option := CommandOption{Type: optionType, Name: name, Description: desc, Required: !optional, Autocomplete: autocomplete}

		for _, bound := range []string{"min", "max"} {
			raw, ok := field.Tag.Lookup(bound)
			if !ok {
				continue
			}
			switch optionType {
			case OptionString:
				n, err := strconv.Atoi(raw)
				if err != nil || n < 0 || n > 6000 {
					fail("%s of %q must be a length from 0 to 6000", bound, name)
				}
				if bound == "min" {
					option.MinLength = &n
				} else {
					option.MaxLength = &n
				}
			case OptionInteger, OptionNumber:
				v, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					fail("%s of %q is not a number", bound, name)
				}
				if bound == "min" {
					option.MinValue = &v
				} else {
					option.MaxValue = &v
				}
			default:
				fail("%s does not apply to %q", bound, name)
			}
		}
		if raw := field.Tag.Get("choices"); raw != "" {
			if autocomplete {
				fail("%q cannot have both choices and autocomplete", name)
			}
			option.Choices = parseChoices(raw, kind, func(format string, args ...any) {
				fail("choices of %q: "+format, append([]any{name}, args...)...)
			})
		}
		if raw := field.Tag.Get("channel"); raw != "" {
			if kind != argChannel {
				fail("channel kinds only apply to *Channel fields")
			}
			for _, part := range strings.Split(raw, ",") {
				ct, ok := channelKinds[strings.TrimSpace(part)]
				if !ok {
					fail("unknown channel kind %q", part)
				}
				option.ChannelTypes = append(option.ChannelTypes, ct...)
			}
		}
		plan.fields = append(plan.fields, argField{index: index, kind: kind, option: option, optional: optional})
	}
	if len(plan.fields) > 25 {
		fail("%d options; Discord allows 25", len(plan.fields))
	}
	// Discord requires required options first. Decoding is by name, so the
	// order in the struct does not matter to the handler.
	slices.SortStableFunc(plan.fields, func(a, b argField) int {
		switch {
		case !a.optional && b.optional:
			return -1
		case a.optional && !b.optional:
			return 1
		}
		return 0
	})
	return plan
}

func checkName(name string, fail func(string, ...any)) {
	if !commandNamePattern.MatchString(name) || strings.ToLower(name) != name {
		fail("%q is not a valid name: use 1-32 lowercase letters, digits, - or _", name)
	}
}

func argKindOf(t reflect.Type) (argKind, OptionType) {
	switch t {
	case durationType:
		return argDuration, OptionString
	case snowflakeType:
		return argSnowflake, OptionMentionable
	case reflect.TypeFor[*User]():
		return argUser, OptionUser
	case reflect.TypeFor[*Member]():
		return argMember, OptionUser
	case reflect.TypeFor[*Role]():
		return argRole, OptionRole
	case reflect.TypeFor[*Channel]():
		return argChannel, OptionChannel
	case reflect.TypeFor[*Attachment]():
		return argAttachment, OptionAttachment
	}
	switch t.Kind() {
	case reflect.String:
		return argString, OptionString
	case reflect.Bool:
		return argBool, OptionBoolean
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return argInt, OptionInteger
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return argUint, OptionInteger
	case reflect.Float32, reflect.Float64:
		return argFloat, OptionNumber
	}
	return 0, 0
}

var channelKinds = map[string][]ChannelType{
	"text":         {ChannelGuildText},
	"voice":        {ChannelGuildVoice},
	"category":     {ChannelGuildCategory},
	"announcement": {ChannelGuildAnnouncement},
	"stage":        {ChannelGuildStageVoice},
	"forum":        {ChannelGuildForum},
	"media":        {ChannelGuildMedia},
	"thread":       {ChannelAnnouncementThread, ChannelPublicThread, ChannelPrivateThread},
}

func parseChoices(raw string, kind argKind, fail func(string, ...any)) []CommandChoice {
	var out []CommandChoice
	for _, pair := range strings.Split(raw, "|") {
		label, value, ok := strings.Cut(pair, "=")
		if !ok {
			label, value = pair, pair
		}
		label, value = strings.TrimSpace(label), strings.TrimSpace(value)
		if label == "" || utf8.RuneCountInString(label) > 100 {
			fail("label %q must be 1-100 characters", label)
		}
		choice := CommandChoice{Name: label}
		switch kind {
		case argString:
			choice.Value = value
		case argDuration:
			if _, err := ParseDuration(value); err != nil {
				fail("%q is not a duration", value)
			}
			choice.Value = value
		case argInt, argUint:
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				fail("%q is not an integer", value)
			}
			choice.Value = n
		case argFloat:
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				fail("%q is not a number", value)
			}
			choice.Value = f
		default:
			fail("choices only apply to text and number fields")
		}
		out = append(out, choice)
	}
	if len(out) > 25 {
		fail("%d choices; Discord allows 25", len(out))
	}
	return out
}

func (p argPlan) options() []CommandOption {
	out := make([]CommandOption, len(p.fields))
	for n, f := range p.fields {
		out[n] = f.option
	}
	return out
}

// decode fills dst from the interaction. Discord enforces required options,
// types, and ranges before the bot sees them; the checks here cover what it
// cannot, such as a user who is not in the server.
func (p argPlan) decode(i *InteractionCreate, dst reflect.Value) error {
	for _, f := range p.fields {
		name := f.option.Name
		opt := i.Data.Option(name)
		if opt == nil {
			if f.optional {
				continue
			}
			return UserErrorf("The %s option is missing.", name)
		}
		field := dst.Field(f.index)
		switch f.kind {
		case argString:
			field.SetString(opt.String())
		case argBool:
			field.SetBool(opt.Bool())
		case argInt:
			field.SetInt(opt.Int())
		case argUint:
			field.SetUint(uint64(max(opt.Int(), 0)))
		case argFloat:
			field.SetFloat(opt.Float())
		case argDuration:
			d, err := ParseDuration(opt.String())
			if err != nil {
				return UserErrorf("%q is not a length of time. Try something like 10m, 2h, or 3d.", opt.String())
			}
			field.SetInt(int64(d))
		case argSnowflake:
			field.SetUint(uint64(opt.Snowflake()))
		case argUser:
			if u := i.UserOption(name); u != nil {
				field.Set(reflect.ValueOf(u))
			}
		case argMember:
			m := i.MemberOption(name)
			if m == nil {
				return UserErrorf("That user is not a member of this server.")
			}
			field.Set(reflect.ValueOf(m))
		case argRole:
			if r := i.RoleOption(name); r != nil {
				field.Set(reflect.ValueOf(r))
			}
		case argChannel:
			if ch := i.ChannelOption(name); ch != nil {
				field.Set(reflect.ValueOf(ch))
			}
		case argAttachment:
			if a := i.AttachmentOption(name); a != nil {
				field.Set(reflect.ValueOf(a))
			}
		}
	}
	return nil
}

// snakeCase turns a Go field name into an option name: UserID -> user_id.
func snakeCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for n, r := range runes {
		if unicode.IsUpper(r) {
			if n > 0 && (unicode.IsLower(runes[n-1]) || (n+1 < len(runes) && unicode.IsLower(runes[n+1]))) {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ParseDuration reads a length of time as people type it in Discord. It
// accepts Go durations such as 90s, 10m, or 1h30m, plus d for days and w for
// weeks, as in 3d or 1w2d. A bare number is seconds.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("starlings: empty duration")
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	var total time.Duration
	for s != "" {
		end := 0
		for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '.') {
			end++
		}
		if end == 0 {
			return 0, fmt.Errorf("starlings: invalid duration %q", s)
		}
		value, err := strconv.ParseFloat(s[:end], 64)
		if err != nil {
			return 0, fmt.Errorf("starlings: invalid duration %q", s)
		}
		s = s[end:]
		unitEnd := 0
		for unitEnd < len(s) && (s[unitEnd] < '0' || s[unitEnd] > '9') && s[unitEnd] != '.' {
			unitEnd++
		}
		unit := strings.TrimSpace(s[:unitEnd])
		s = strings.TrimSpace(s[unitEnd:])
		var scale time.Duration
		switch unit {
		case "w":
			scale = 7 * 24 * time.Hour
		case "d":
			scale = 24 * time.Hour
		case "h":
			scale = time.Hour
		case "m":
			scale = time.Minute
		case "s":
			scale = time.Second
		case "ms":
			scale = time.Millisecond
		default:
			return 0, fmt.Errorf("starlings: unknown duration unit %q", unit)
		}
		total += time.Duration(value * float64(scale))
	}
	return total, nil
}
