package domain

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

// Badge stores a venue badge.
type Badge struct {
	Text    string `json:"text"`
	Variant string `json:"variant"`
}

// Venue stores discovery item venue details.
type Venue struct {
	ID               any      `json:"id"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Address          string   `json:"address"`
	Badges           []Badge  `json:"badges"`
	Promotions       []any    `json:"promotions"`
	Country          string   `json:"country"`
	Currency         string   `json:"currency"`
	Delivers         bool     `json:"delivers"`
	DeliveryPriceInt *int     `json:"delivery_price_int"`
	EstimateRange    string   `json:"estimate_range"`
	Estimate         float64  `json:"estimate"`
	Icon             string   `json:"icon"`
	Online           *bool    `json:"online"`
	ProductLine      string   `json:"product_line"`
	ShowWoltPlus     bool     `json:"show_wolt_plus"`
	Tags             []string `json:"tags"`
	Rating           *Rating  `json:"rating"`
	PriceRange       int      `json:"price_range"`
}

// Link stores item link metadata.
type Link struct {
	Target string `json:"target"`
}

// Item stores discovery items and menu placeholders.
type Item struct {
	Title   string `json:"title"`
	TrackID string `json:"track_id"`
	Link    Link   `json:"link"`
	Venue   *Venue `json:"venue"`
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

// Times stores opening/closing values.
type Times struct {
	Type  string           `json:"type"`
	Value map[string]int64 `json:"value"`
}

// Statistics stores min/max/mean details for estimates.
type Statistics struct {
	Mean *int `json:"mean"`
	Max  *int `json:"max"`
	Min  *int `json:"min"`
}

// Estimates stores delivery estimate sections.
type Estimates struct {
	Delivery    Statistics `json:"delivery"`
	Pickup      Statistics `json:"pickup"`
	Preparation Statistics `json:"preparation"`
	Total       Statistics `json:"total"`
}

// Restaurant stores the detailed venue payload.
type Restaurant struct {
	ID                    any                `json:"id"`
	Slug                  string             `json:"slug"`
	Name                  []Translation      `json:"name"`
	Address               string             `json:"address"`
	City                  string             `json:"city"`
	Country               string             `json:"country"`
	Currency              string             `json:"currency"`
	FoodTags              []string           `json:"food_tags"`
	Phone                 string             `json:"phone"`
	PriceRange            int                `json:"price_range"`
	PublicURL             string             `json:"public_url"`
	Rating                *RatingDetail      `json:"rating"`
	Website               string             `json:"website"`
	AllowedPaymentMethods []string           `json:"allowed_payment_methods"`
	Description           []Translation      `json:"description"`
	ShortDescription      []Translation      `json:"short_description"`
	Estimates             *Estimates         `json:"estimates"`
	OpeningTimes          map[string][]Times `json:"opening_times"`
	DeliveryMethods       []string           `json:"delivery_methods"`
	TimezoneName          string             `json:"timezone_name"`
}
