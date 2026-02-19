package domain

// Profile stores user location settings.
type Profile struct {
	Name      string   `json:"name"`
	IsDefault bool     `json:"is_default"`
	Address   string   `json:"address"`
	Location  Location `json:"location"`
}

// Config stores all local profiles.
type Config struct {
	Profiles []Profile `json:"profiles"`
}
