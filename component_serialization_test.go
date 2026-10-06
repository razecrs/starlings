package starlings

import (
	"bytes"
	json "encoding/json/v2"
	"testing"
)

func TestTextInputOmitsUnsetLengthLimits(t *testing.T) {
	response := InteractionResponse{
		Type: CallbackModal,
		Data: &InteractionResponseData{
			CustomID: "idea:create",
			Title:    "Create idea",
			Components: []Component{
				Label("Idea", "", TextInput(TextInputParagraph, "idea:text", "Drop something useful.", true)),
			},
		},
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range [][]byte{[]byte(`"min_length"`), []byte(`"max_length"`)} {
		if bytes.Contains(payload, field) {
			t.Fatalf("unset %s was serialized: %s", field, payload)
		}
	}
}

func TestTextInputIncludesSetLengthLimits(t *testing.T) {
	minLength := 1
	component := TextInput(TextInputShort, "name", "Your name", true)
	component.MinLength = &minLength
	component.MaxLength = 100

	payload, err := json.Marshal(component)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range [][]byte{[]byte(`"min_length":1`), []byte(`"max_length":100`)} {
		if !bytes.Contains(payload, field) {
			t.Fatalf("set %s is missing: %s", field, payload)
		}
	}
}

func TestOptionalScalarsUseJSONV2ZeroSemantics(t *testing.T) {
	payload, err := json.Marshal(struct {
		Component Component  `json:"component"`
		Embed     Embed      `json:"embed"`
		Reference MessageRef `json:"reference"`
	}{})
	if err != nil {
		t.Fatal(err)
	}

	for _, field := range [][]byte{
		[]byte(`"max_values"`),
		[]byte(`"disabled"`),
		[]byte(`"color"`),
		[]byte(`"inline"`),
		[]byte(`"channel_id"`),
	} {
		if bytes.Contains(payload, field) {
			t.Fatalf("zero-valued optional field %s was serialized: %s", field, payload)
		}
	}
}
