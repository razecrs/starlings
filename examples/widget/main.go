// Command widget drives Discord's Game Stats Widget API with starlings.
//
//	go run ./examples/widget info    # application, owner, and the authorise URL
//	go run ./examples/widget push    # write stats to a player's profile
//	go run ./examples/widget get     # read them back
//
// The API is gated: the application needs the Social SDK enabled and a claimed
// game, and the player must have authorised it with the
// application_identities.write scope. See identity.go for the details.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/razecrs/starlings"
)

func main() {
	log.SetFlags(0)

	var (
		envPath  = flag.String("env", "", "path to a .env file")
		userFlag = flag.String("user", "", "the player's Discord user ID")
		gameID   = flag.String("game-id", "player-1", "the player's ID in your own system (provider_issued_user_id)")
		redirect = flag.String("redirect", "https://discord.com", "OAuth2 redirect URI")
	)

	args := os.Args[1:]
	cmd := "info"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	if err := flag.CommandLine.Parse(args); err != nil {
		log.Fatal(err)
	}
	if flag.NArg() > 0 && cmd == "info" {
		cmd = flag.Arg(0)
	}

	token, err := loadToken(*envPath)
	if err != nil {
		log.Fatal(err)
	}

	bot := starlings.New(token)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := bot.CurrentApplication(ctx)
	if err != nil {
		log.Fatalf("looking up the application (is the token valid?): %v", err)
	}

	switch cmd {
	case "info":
		info(app, *redirect)
	case "push":
		push(ctx, bot, app, mustUser(*userFlag, app), *gameID)
	case "get":
		get(ctx, bot, app, mustUser(*userFlag, app), *gameID)
	default:
		log.Fatalf("unknown command %q - use info, push or get", cmd)
	}
}

func info(app *starlings.CurrentApplication, redirect string) {
	fmt.Printf("application : %s (%s)\n", app.Name, app.ID)
	if app.Team != nil {
		fmt.Printf("team        : %q (%s)\n", app.Team.Name, app.Team.ID)
	}
	fmt.Printf("human owner : %s\n\n", app.HumanOwnerID())

	fmt.Println("The player authorises here, then you can write to their profile:")
	fmt.Println()
	fmt.Println(" ", starlings.AuthorizeURL(app.ID, redirect,
		starlings.ScopeApplicationIdentitiesWrite))
	fmt.Println()
	fmt.Println("The redirect URI must be registered under OAuth2 in the portal.")
	if id := app.HumanOwnerID(); !id.IsZero() {
		fmt.Printf("\nThen: go run ./examples/widget -user %s push\n", id)
	}
}

func push(ctx context.Context, bot *starlings.Client, app *starlings.CurrentApplication, userID starlings.Snowflake, gameID string) {
	// Primary fields are Discord's generic set; dynamic fields cover anything
	// else, including images - which is how a widget tile gets a thumbnail
	// next to its text.
	profile := starlings.IdentityProfile{
		Username: "player-one",
		Data: &starlings.ProfileData{
			Primary: &starlings.PrimaryProfileData{
				RankName:      "Bug Hunter",
				HighestRank:   "Bug Hunter",
				PlaytimeHours: 412.5,
				TotalWins:     1204,
				TotalGames:    1337,
			},
			Dynamic: []starlings.DynamicField{
				starlings.StringField("project_1_name", "Starship"),
				starlings.StringField("project_1_desc", "A sample project."),
				starlings.MediaField("project_1_icon", "https://example.com/starship.png"),
				starlings.NumberField("commits", 12847),
			},
		},
	}

	out, err := bot.SetIdentityProfile(ctx, app.ID, userID, gameID, profile)
	if err != nil {
		log.Fatalf("writing the identity profile: %v\n\n"+
			"401/403 here usually means the player has not authorised the\n"+
			"application with the %s scope, or the game is not claimed.",
			err, starlings.ScopeApplicationIdentitiesWrite)
	}
	fmt.Printf("wrote profile for %s (game id %s): username=%q\n", userID, gameID, out.Username)
}

func get(ctx context.Context, bot *starlings.Client, app *starlings.CurrentApplication, userID starlings.Snowflake, gameID string) {
	ids, err := bot.UserApplicationIdentities(ctx, userID, app.ID)
	if err != nil {
		fmt.Println("listing identities:", err)
	} else {
		fmt.Printf("identities: %+v\n", ids)
	}

	profile, err := bot.IdentityProfile(ctx, app.ID, userID, gameID)
	if err != nil {
		log.Fatalf("reading the identity profile: %v", err)
	}
	fmt.Printf("username: %q\n", profile.Username)
	if profile.Data != nil {
		fmt.Printf("primary : %+v\n", profile.Data.Primary)
		for _, d := range profile.Data.Dynamic {
			fmt.Printf("dynamic : %s = %v (type %d)\n", d.Name, d.Value, d.Type)
		}
	}
}

func mustUser(flagValue string, app *starlings.CurrentApplication) starlings.Snowflake {
	if flagValue != "" {
		id, err := starlings.ParseSnowflake(flagValue)
		if err != nil {
			log.Fatalf("bad -user value: %v", err)
		}
		return id
	}
	if id := app.HumanOwnerID(); !id.IsZero() {
		return id
	}
	log.Fatal("pass -user with the player's Discord user ID")
	return 0
}

// loadToken prefers DISCORD_TOKEN and reads a file only when -env names it.
func loadToken(explicit string) (string, error) {
	if t := os.Getenv("DISCORD_TOKEN"); t != "" {
		return t, nil
	}

	if explicit == "" {
		return "", fmt.Errorf("set DISCORD_TOKEN or pass -env with an explicit file")
	}

	vals, err := readEnv(explicit)
	if err != nil {
		return "", err
	}
	if t := vals["DISCORD_TOKEN"]; t != "" {
		return t, nil
	}
	return "", fmt.Errorf("no DISCORD_TOKEN in %s", explicit)
}

// readEnv parses the small subset of .env syntax worth supporting: KEY=VALUE
// lines, # comments, and optional surrounding quotes.
func readEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return vals, sc.Err()
}
