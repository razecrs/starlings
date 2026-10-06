package starlings

import "testing"

func TestCurrentPermissionBits(t *testing.T) {
	tests := map[string]struct {
		got  Permissions
		want Permissions
	}{
		"soundboard":         {PermissionUseSoundboard, 1 << 42},
		"create expressions": {PermissionCreateGuildExpressions, 1 << 43},
		"create events":      {PermissionCreateEvents, 1 << 44},
		"external sounds":    {PermissionUseExternalSounds, 1 << 45},
		"voice messages":     {PermissionSendVoiceMessages, 1 << 46},
		"voice status":       {PermissionSetVoiceChannelStatus, 1 << 48},
		"polls":              {PermissionSendPolls, 1 << 49},
		"external apps":      {PermissionUseExternalApps, 1 << 50},
		"pin messages":       {PermissionPinMessages, 1 << 51},
		"bypass slowmode":    {PermissionBypassSlowmode, 1 << 52},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("permission = %d, want %d", test.got, test.want)
			}
		})
	}
}

func TestHighPermissionString(t *testing.T) {
	if got, want := PermissionBypassSlowmode.String(), "4503599627370496"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
