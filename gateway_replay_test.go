package starlings

import (
	"context"
	"testing"
)

func TestHandleGatewayFrameUsesNormalDispatch(t *testing.T) {
	client := testClient()
	var got *MessageCreate
	On(client, func(event *MessageCreate) { got = event })
	if err := client.HandleGatewayFrame(context.Background(), []byte(messageCreateFrame)); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Content != "hello there" {
		t.Fatalf("event=%#v", got)
	}
}

func TestHandleGatewayFrameValidatesInput(t *testing.T) {
	var client *Client
	if err := client.HandleGatewayFrame(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("nil client accepted")
	}
	client = testClient()
	if err := client.HandleGatewayFrame(context.Background(), nil); err == nil {
		t.Fatal("empty frame accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.HandleGatewayFrame(ctx, []byte(`{}`)); err == nil {
		t.Fatal("cancelled context accepted")
	}
}
