package domain

// Location identifies a point on earth.
type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}
