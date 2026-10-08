package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/razecrs/starlings"
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[91m"
	green  = "\x1b[92m"
	yellow = "\x1b[93m"
	cyan   = "\x1b[96m"
)

type lookupResult struct {
	ID       starlings.Snowflake
	Username string
	Found    bool
}

func main() {
	plain := flag.Bool("plain", false, "disable ANSI colours for files and CI")
	runs := flag.Int("runs", 400, "comparison samples per performance case")
	flag.Parse()
	if *runs < 1 {
		*runs = 1
	}
	colour := !*plain && os.Getenv("NO_COLOR") == ""
	paint := func(code, text string) string {
		if !colour {
			return text
		}
		return code + text + reset
	}

	fmt.Println(paint(bold+cyan, "✦ STARLINGS GUARD — manual code under pressure"))
	fmt.Println(paint(dim, "  same inputs · both implementations measured · no Discord connection"))

	members := make([]starlings.Member, 25_000)
	for index := range members {
		id := starlings.Snowflake(index + 10)
		members[index] = starlings.Member{User: &starlings.User{ID: id, Username: fmt.Sprintf("member-%d", index)}}
	}
	target := members[len(members)-1]
	target.Roles = []starlings.Snowflake{2}
	members[len(members)-1] = target

	bot := starlings.New(starlings.WithToken("offline-demo-token"),
		starlings.WithStateMode(starlings.StateManual),
		starlings.WithGuard(),
		starlings.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	if err := bot.State.Apply(&starlings.GuildCreate{Guild: starlings.Guild{
		ID: 1,
		Roles: []starlings.Role{
			{ID: 1, Name: "@everyone", Permissions: starlings.PermissionViewChannel | starlings.PermissionSendMessages},
			{ID: 2, Name: "member", Permissions: starlings.PermissionAttachFiles},
		},
		Members: members,
		Channels: []starlings.Channel{{
			ID: 20, GuildID: 1, Name: "guard-demo",
			PermissionOverwrites: []starlings.Overwrite{{
				ID: target.User.ID, Type: 1, Deny: starlings.PermissionSendMessages,
			}},
		}},
	}}); err != nil {
		log.Fatal(err)
	}

	section(paint, "01", "correctness", "manual permissions forget a member overwrite")
	manualPermissions := func() (starlings.Permissions, error) {
		return starlings.PermissionViewChannel | starlings.PermissionSendMessages | starlings.PermissionAttachFiles, nil
	}
	automaticPermissions := func() (starlings.Permissions, error) {
		return bot.State.ChannelPermissions(20, target.User.ID)
	}
	var manualPerms starlings.Permissions
	for range 24 {
		var err error
		manualPerms, err = starlings.GuardCompare(bot.Guard(), "permissions.resolve",
			func(a, b starlings.Permissions) bool { return a == b }, manualPermissions, automaticPermissions)
		if err != nil {
			log.Fatal(err)
		}
	}
	automaticPerms, err := automaticPermissions()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  manual send=%-5v  automatic send=%-5v  %s\n",
		manualPerms.Has(starlings.PermissionSendMessages),
		automaticPerms.Has(starlings.PermissionSendMessages),
		paint(red, "MISMATCH CAUGHT"))

	section(paint, "02", "performance", "linear member scan versus the state index")
	manualLookup := func() (lookupResult, error) {
		var result lookupResult
		for range 16 {
			for _, member := range members {
				if member.User != nil && member.User.ID == target.User.ID {
					result = lookupResult{ID: member.User.ID, Username: member.User.Username, Found: true}
					break
				}
			}
		}
		return result, nil
	}
	automaticLookup := func() (lookupResult, error) {
		var result lookupResult
		for range 16 {
			member, found := bot.State.Member(1, target.User.ID)
			if !found || member.User == nil {
				result = lookupResult{Found: false}
				continue
			}
			result = lookupResult{ID: member.User.ID, Username: member.User.Username, Found: true}
		}
		return result, nil
	}
	for range *runs {
		_, err = starlings.GuardCompare(bot.Guard(), "members.lookup", nil, manualLookup, automaticLookup)
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("  compared %s members across %s samples  %s\n",
		comma(len(members)), comma(*runs), paint(green, "OUTPUTS MATCH"))

	section(paint, "03", "stability", "production samples can compare paths without replaying them")
	manualFailure := errors.New("manual decoder rejected a valid optional field")
	for sample := range 12 {
		var manualErr error
		if sample == 2 || sample == 9 {
			manualErr = manualFailure
		}
		bot.Guard().Observe("payload.decode", starlings.GuardManual, time.Duration(150+sample)*time.Microsecond, manualErr)
		bot.Guard().Observe("payload.decode", starlings.GuardAutomatic, time.Duration(65+sample)*time.Microsecond, nil)
	}
	fmt.Println("  manual errors=2     automatic errors=0      " + paint(green, "SAFER PATH FOUND"))

	section(paint, "04", "side effects", "Measure executes sends, writes, and moderation actions once")
	writes := 0
	err = bot.Guard().Measure("audit.write", starlings.GuardManual, func() error {
		writes++
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  audit writes=%d  %s\n", writes, paint(yellow, "MEASURED, NEVER REPLAYED"))

	printReport(bot.Guard().Report(), paint)
	fmt.Println()
	fmt.Println(paint(bold+green, "Guard verdict: automatic wins where it is safer or faster; manual stays available when its control is worth it."))
}

func section(paint func(string, string) string, number, title, detail string) {
	fmt.Println()
	fmt.Printf("%s  %s\n", paint(bold+cyan, number+"  "+strings.ToUpper(title)), paint(dim, detail))
}

func printReport(report starlings.GuardReport, paint func(string, string) string) {
	type pair struct {
		manual, automatic *starlings.GuardMetric
	}
	rows := make(map[string]*pair)
	for index := range report.Metrics {
		metric := &report.Metrics[index]
		if rows[metric.Feature] == nil {
			rows[metric.Feature] = &pair{}
		}
		if metric.Implementation == starlings.GuardManual {
			rows[metric.Feature].manual = metric
		} else {
			rows[metric.Feature].automatic = metric
		}
	}

	fmt.Println()
	fmt.Println(paint(bold+cyan, "┌─ GUARD REPORT ─────────────────────────────────────────────────────────────────────────────┐"))
	fmt.Printf("%-24s %-19s %-19s %-10s %s\n", "FEATURE", "MANUAL", "AUTOMATIC", "CHECKS", "VERDICT")
	for _, advice := range report.Advice {
		row := rows[advice.Feature]
		manual := metricText(row.manual)
		automatic := metricText(row.automatic)
		verdictColour := cyan
		switch {
		case advice.Mismatches > 0:
			verdictColour = red
		case strings.Contains(advice.Recommendation, "automatic"):
			verdictColour = green
		case strings.Contains(advice.Recommendation, "manual path"):
			verdictColour = yellow
		}
		checks := fmt.Sprintf("%d/%d bad", advice.Mismatches, advice.Checks)
		fmt.Printf("%-24s %-19s %-19s %-10s %s\n",
			advice.Feature, manual, automatic, checks, paint(verdictColour, advice.Recommendation))
	}
	fmt.Println(paint(bold+cyan, "└───────────────────────────────────────────────────────────────────────────────────────────┘"))
}

func metricText(metric *starlings.GuardMetric) string {
	if metric == nil {
		return "—"
	}
	return fmt.Sprintf("%s · %d err", shortDuration(metric.Average), metric.Errors)
}

func shortDuration(duration time.Duration) string {
	if duration == 0 {
		return "<1ns"
	}
	if duration >= time.Millisecond {
		return duration.Round(10 * time.Microsecond).String()
	}
	if duration >= time.Microsecond {
		return duration.Round(time.Microsecond).String()
	}
	return duration.String()
}

func comma(value int) string {
	digits := strconv.Itoa(value)
	for position := len(digits) - 3; position > 0; position -= 3 {
		digits = digits[:position] + "," + digits[position:]
	}
	return digits
}
