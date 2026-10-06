package starlings

import (
	"context"
	"net/http"
)

// Widget configs describe how a Game Stats Widget looks: which layout each
// surface uses, and where every field gets its value.
//
// None of this is documented by Discord - the shapes below were recovered by
// reading live published configs off the API. See docs/game-stats-widgets.md
// for the full vocabulary, including which components each layout expects.
//
// Creating a config requires a claimed game on the application's team and
// fails with error 40128 otherwise. Reading and editing an existing config
// have no such gate, and reading works across applications: any bot token can
// fetch any application's published config.

// WidgetValueType says where a field's content comes from.
const (
	// WidgetData pulls from the player's identity profile; the field's value
	// is the key to read. Discord's editor calls this "User Data".
	WidgetData = "data"
	// WidgetCustomString is a literal, identical for every player. Max 256.
	WidgetCustomString = "custom_string"
	// WidgetApplicationAsset is an image uploaded to the developer portal;
	// the value is its asset key. The asset must be set Public.
	WidgetApplicationAsset = "application_asset"
)

// WidgetPresentationType is how a field renders.
const (
	PresentText     = "text"     // as-is; numbers are not rendered
	PresentNumber   = "number"   // compact notation, 1500 -> 1.5K
	PresentDuration = "duration" // milliseconds -> "6h 30m"
	PresentImage    = "image"    // image fields only
)

// Known layout names, one per surface.
const (
	LayoutTopHero          = "widget_top_hero"          // hero_image, title, subtitle_1..3
	LayoutBottomStats      = "widget_bottom_stats"      // stat_1..stat_6, each value+label
	LayoutBottomCollection = "widget_bottom_collection" // item_1..item_4, each image+name+description
	LayoutPreviewHero      = "add_widget_preview_hero"  // hero_image
	LayoutMiniProfileStat  = "mini_profile_hero_stat"   // hero_image, stat
)

// WidgetField is one displayed value.
type WidgetField struct {
	ValueType        string `json:"value_type"`
	PresentationType string `json:"presentation_type"`
	Value            string `json:"value"`
}

// DataField reads a key from the player's identity profile.
func DataField(key, presentation string) WidgetField {
	return WidgetField{ValueType: WidgetData, PresentationType: presentation, Value: key}
}

// LiteralField is a fixed string, the same for every player.
func LiteralField(text string) WidgetField {
	return WidgetField{ValueType: WidgetCustomString, PresentationType: PresentText, Value: text}
}

// AssetField references an image uploaded to the developer portal by its key.
func AssetField(assetKey string) WidgetField {
	return WidgetField{ValueType: WidgetApplicationAsset, PresentationType: PresentImage, Value: assetKey}
}

// WidgetComponent is one slot in a layout - a stat, a tile, the title.
type WidgetComponent struct {
	Fields map[string]WidgetField `json:"fields"`
}

// WidgetSurface is one region of the widget: a layout plus its components.
type WidgetSurface struct {
	Layout     string                     `json:"layout"`
	Components map[string]WidgetComponent `json:"components"`
}

// WidgetAsset is an image uploaded to the application, as reported back
// alongside a config.
type WidgetAsset struct {
	Key        string    `json:"key"`
	AssetID    Snowflake `json:"asset_id"`
	AssetType  string    `json:"asset_type"`
	Visibility string    `json:"visibility"` // "public" or the image renders as a skeleton
	Metadata   struct {
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		ContentType string `json:"content_type"`
		IsAnimated  bool   `json:"is_animated"`
	} `json:"metadata"`
}

// WidgetConfig is one widget's appearance. Surfaces are keyed "widget_top",
// "widget_bottom", "add_widget_preview", "mini_profile" and
// "activity_accessory"; the first three are required before publishing.
type WidgetConfig struct {
	ConfigID       string                   `json:"config_id,omitzero"`
	ApplicationID  Snowflake                `json:"application_id,omitzero"`
	DisplayName    string                   `json:"display_name,omitzero"`
	Status         string                   `json:"status,omitzero"` // "published" once live
	Surfaces       map[string]WidgetSurface `json:"surfaces,omitzero"`
	ResolvedAssets []WidgetAsset            `json:"resolved_assets,omitzero"`
}

// WidgetConfigs lists an application's widget configs.
//
// This works for any application, not just the token's own, which is the only
// practical way to inspect a layout Discord does not document.
func (c *Client) WidgetConfigs(ctx context.Context, appID Snowflake) ([]WidgetConfig, error) {
	var out []WidgetConfig
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/widget-configs",
		Route:  "GET /applications/{id}/widget-configs",
	}, &out)
	return out, err
}

// CreateWidgetConfig registers a new widget config and returns its ID.
//
// Requires a claimed game on the application's team; without one Discord
// returns 40128, and that gate applies even to applications that already have
// a published config from before it was introduced.
func (c *Client) CreateWidgetConfig(ctx context.Context, appID Snowflake, displayName string) (*WidgetConfig, error) {
	var out WidgetConfig
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/widget-configs",
		Route:  "POST /applications/{id}/widget-configs",
		Body:   map[string]string{"display_name": displayName},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateWidgetConfig rewrites a config's surfaces.
//
// Send every surface you want to keep, not only the ones being changed:
// "surfaces" is replaced wholesale, so omitting a required surface unpublishes
// the widget.
func (c *Client) UpdateWidgetConfig(ctx context.Context, appID Snowflake, configID string, surfaces map[string]WidgetSurface) (*WidgetConfig, error) {
	var out WidgetConfig
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/applications/" + appID.String() + "/widget-configs/" + configID,
		Route:  "PATCH /applications/{id}/widget-configs/{id}",
		Body:   map[string]any{"surfaces": surfaces},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PublishWidgetConfig makes a config live. Widget Top, Widget Bottom and Add
// Widget Preview must all have a layout and their required fields set.
func (c *Client) PublishWidgetConfig(ctx context.Context, appID Snowflake, configID string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/widget-configs/" + configID + "/publish",
		Route:  "POST /applications/{id}/widget-configs/{id}/publish",
	}, nil)
}

// StatsSurface builds a widget_bottom_stats surface from up to six stats, each
// a profile key rendered with the given presentation and a fixed label.
func StatsSurface(stats ...WidgetStat) WidgetSurface {
	const slots = 6
	comps := make(map[string]WidgetComponent, min(len(stats), slots))
	for i, s := range stats {
		if i >= slots {
			break
		}
		presentation := s.Presentation
		if presentation == "" {
			presentation = PresentText
		}
		comps["stat_"+itoa(i+1)] = WidgetComponent{Fields: map[string]WidgetField{
			"value": DataField(s.Key, presentation),
			"label": LiteralField(s.Label),
		}}
	}
	return WidgetSurface{Layout: LayoutBottomStats, Components: comps}
}

// WidgetStat is one entry of a stats grid.
type WidgetStat struct {
	Key          string // profile data key, e.g. "total_wins"
	Label        string // the caption under it
	Presentation string // PresentText, PresentNumber or PresentDuration
}

// CollectionSurface builds a widget_bottom_collection surface - four tiles,
// each an image with a name and a description, all read from profile data.
//
// keyPrefix is combined with the tile number, so a prefix of "item" reads
// item_1_image, item_1_name, item_1_desc and so on.
func CollectionSurface(keyPrefix string) WidgetSurface {
	const slots = 4
	comps := make(map[string]WidgetComponent, slots)
	for i := 1; i <= slots; i++ {
		n := keyPrefix + "_" + itoa(i)
		comps["item_"+itoa(i)] = WidgetComponent{Fields: map[string]WidgetField{
			"image":       DataField(n+"_image", PresentImage),
			"name":        DataField(n+"_name", PresentText),
			"description": DataField(n+"_desc", PresentText),
		}}
	}
	return WidgetSurface{Layout: LayoutBottomCollection, Components: comps}
}
