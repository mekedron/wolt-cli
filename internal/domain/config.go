package domain

// Profile stores user location settings.
type Profile struct {
	Name          string   `json:"name"`
	IsDefault     bool     `json:"is_default"`
	Location      Location `json:"location"`
	WToken        string   `json:"wtoken,omitempty"`
	WRefreshToken string   `json:"wrefresh_token,omitempty"`
	Cookies       []string `json:"cookies,omitempty"`
	WoltAddressID string   `json:"wolt_address_id,omitempty"`
}

// Config stores all local profiles.
type Config struct {
	Profiles []Profile `json:"profiles"`
}
