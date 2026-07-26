package domain

import "strings"

// Rating stores public venue score.
type Rating struct {
	Rating float64 `json:"rating"`
	Score  float64 `json:"score"`
}

// RatingDetail stores long-form venue rating details.
type RatingDetail struct {
	NegativePercentage int     `json:"negative_percentage"`
	NeutralPercentage  int     `json:"neutral_percentage"`
	PositivePercentage int     `json:"positive_percentage"`
	Rating             int     `json:"rating"`
	Score              float64 `json:"score"`
	Text               string  `json:"text"`
	Volume             int     `json:"volume"`
}

// Badge stores a venue badge. Icon is populated for the BadgesV2 shape;
// the legacy Badges payload omits it, so it deserializes as "".
type Badge struct {
	Text    string `json:"text"`
	Variant string `json:"variant"`
	Icon    string `json:"icon"`
}

// Venue stores discovery item venue details.
type Venue struct {
	ID                     any            `json:"id"`
	Slug                   string         `json:"slug"`
	Name                   string         `json:"name"`
	Address                string         `json:"address"`
	Badges                 []Badge        `json:"badges"`
	BadgesV2               []Badge        `json:"badges_v2"`
	Promotions             []any          `json:"promotions"`
	PromotionsForTelemetry []any          `json:"promotions_for_telemetry"`
	Country                string         `json:"country"`
	Currency               string         `json:"currency"`
	Delivers               *bool          `json:"delivers"`
	DeliveryPriceInt       *int           `json:"delivery_price_int"`
	EstimateRange          string         `json:"estimate_range"`
	Estimate               float64        `json:"estimate"`
	Icon                   string         `json:"icon"`
	Online                 *bool          `json:"online"`
	ProductLine            string         `json:"product_line"`
	ShowWoltPlus           bool           `json:"show_wolt_plus"`
	ShortDescription       string         `json:"short_description"`
	ShortDescriptionV2     *Translation   `json:"short_description_v2"`
	Status                 map[string]any `json:"status"`
	Tags                   []string       `json:"tags"`
	Rating                 *Rating        `json:"rating"`
	PriceRange             int            `json:"price_range"`
	PreviewItems           []any          `json:"venue_preview_items"`
}

// Tagline returns the venue's marketing one-liner, preferring the localized
// short_description_v2 payload over the legacy short_description string.
func (v *Venue) Tagline() string {
	if v == nil {
		return ""
	}
	if v.ShortDescriptionV2 != nil {
		if value := strings.TrimSpace(v.ShortDescriptionV2.Value); value != "" {
			return value
		}
	}
	return strings.TrimSpace(v.ShortDescription)
}

// Link stores item link metadata.
type Link struct {
	Target string `json:"target"`
}

// Item stores discovery items and menu placeholders.
type Item struct {
	Title     string         `json:"title"`
	TrackID   string         `json:"track_id"`
	Link      Link           `json:"link"`
	OverlayV2 map[string]any `json:"overlay_v2"`
	Venue     *Venue         `json:"venue"`
}

// Section stores front-page sections.
type Section struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Items []Item `json:"items"`
}

// Translation stores localized text fields.
type Translation struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}
