package app

import (
	"errors"
	"io"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestQRChannelTerminalStates(t *testing.T) {
	pairErr := errors.New("pair failed")
	for _, test := range []struct {
		name    string
		events  []whatsmeow.QRChannelItem
		wantErr bool
	}{
		{name: "closed without success", wantErr: true},
		{name: "success", events: []whatsmeow.QRChannelItem{{Event: "success"}}},
		{name: "timeout", events: []whatsmeow.QRChannelItem{{Event: "timeout"}}, wantErr: true},
		{name: "explicit error", events: []whatsmeow.QRChannelItem{{Event: "error", Error: pairErr}}, wantErr: true},
		{name: "unexpected terminal event", events: []whatsmeow.QRChannelItem{{Event: "unexpected"}}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			channel := make(chan whatsmeow.QRChannelItem, len(test.events))
			for _, event := range test.events {
				channel <- event
			}
			close(channel)
			err := waitForQRLogin(channel, io.Discard)
			if (err != nil) != test.wantErr {
				t.Fatalf("waitForQRLogin error = %v, wantErr %v", err, test.wantErr)
			}
			if test.name == "explicit error" && !errors.Is(err, pairErr) {
				t.Fatalf("pairing error was not preserved: %v", err)
			}
		})
	}
}
