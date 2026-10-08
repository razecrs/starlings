package starlings

import (
	"errors"
	"testing"
	"time"
)

// moderationState builds guild 1 owned by user 10, with the bot as user 99.
// Role positions: 2 -> 5 (mod), 3 -> 3 (member), 4 -> 8 (bot).
func moderationState(t *testing.T, botRole Snowflake) *State {
	t.Helper()
	s := newStateWithConfig(StateConfig{Mode: StateManual, Guilds: true, Members: true, Roles: true})
	apply := func(e Event) {
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	apply(&Ready{User: &User{ID: 99}})
	apply(&GuildCreate{Guild: Guild{
		ID: 1, OwnerID: 10,
		Roles: []Role{
			{ID: 1, Position: 0},
			{ID: 2, Position: 5, Permissions: PermissionBanMembers},
			{ID: 3, Position: 3},
			{ID: 4, Position: 8},
		},
		Members: []Member{
			{User: &User{ID: 10}},
			{User: &User{ID: 20}, Roles: []Snowflake{2}},
			{User: &User{ID: 30}, Roles: []Snowflake{3}},
			{User: &User{ID: 31}, Roles: []Snowflake{2}},
			{User: &User{ID: 99}, Roles: []Snowflake{botRole}},
		},
	}})
	return s
}

func TestCanModerateAppliesHierarchy(t *testing.T) {
	s := moderationState(t, 4)
	cases := []struct {
		actor, target Snowflake
		want          ModerationDenial
	}{
		{20, 30, 0},
		{10, 20, 0},
		{20, 20, DenyTargetIsSelf},
		{20, 10, DenyTargetIsOwner},
		{20, 99, DenyTargetIsBot},
		{30, 20, DenyActorTooLow},
		{20, 31, DenyActorTooLow}, // equal positions are not enough
	}
	for _, tc := range cases {
		err := s.CanModerate(1, tc.actor, tc.target)
		var got ModerationDenial
		var denial *ModerationError
		if errors.As(err, &denial) {
			got = denial.Reason
		} else if err != nil {
			t.Fatalf("CanModerate(%d, %d) = %v", tc.actor, tc.target, err)
		}
		if got != tc.want {
			t.Errorf("CanModerate(%d, %d) = %v, want %v", tc.actor, tc.target, got, tc.want)
		}
	}
}

func TestCanModerateChecksBotRole(t *testing.T) {
	s := moderationState(t, 3) // bot's role is level with the target's
	err := s.CanModerate(1, 20, 30)
	var denial *ModerationError
	if !errors.As(err, &denial) || denial.Reason != DenyBotTooLow {
		t.Fatalf("CanModerate with a low bot role = %v, want DenyBotTooLow", err)
	}
	if !IsModerationDenial(err) {
		t.Fatal("IsModerationDenial did not recognise the refusal")
	}
	if err := s.CanModerate(1, 20, 12345); !errors.Is(err, ErrMemberNotCached) || IsModerationDenial(err) {
		t.Fatalf("unknown target = %v, want ErrMemberNotCached", err)
	}
}

func TestTimedOutMembersLosePermissions(t *testing.T) {
	s := moderationState(t, 4)
	until := time.Now().Add(time.Hour)
	if err := s.Apply(&GuildMemberUpdate{GuildID: 1, User: &User{ID: 20}, Roles: []Snowflake{2}, CommunicationDisabledUntil: &until}); err != nil {
		t.Fatal(err)
	}
	perms, err := s.BasePermissions(1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if perms.Has(PermissionBanMembers) {
		t.Fatalf("timed-out member kept Ban Members: %s", perms.HumanString())
	}
	var denial *ModerationError
	if err := s.CanModerate(1, 20, 30); !errors.As(err, &denial) || denial.Reason != DenyActorTimedOut {
		t.Fatalf("timed-out moderator = %v, want DenyActorTimedOut", err)
	}
	owner, _ := s.BasePermissions(1, 10)
	if owner != ^Permissions(0) {
		t.Fatal("the owner should keep every permission")
	}

	// Discord sends null when the timeout is lifted; the cache must follow.
	if err := s.Apply(&GuildMemberUpdate{GuildID: 1, User: &User{ID: 20}, Roles: []Snowflake{2}}); err != nil {
		t.Fatal(err)
	}
	if perms, _ := s.BasePermissions(1, 20); !perms.Has(PermissionBanMembers) {
		t.Fatal("lifting the timeout did not restore the member's permissions")
	}
}
