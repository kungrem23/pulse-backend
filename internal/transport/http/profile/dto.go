package profile

type updateRequest struct {
	Name      string `json:"name"`
	Bio       string `json:"bio"`
	BirthDate string `json:"birth_date"`
	Sex       string `json:"sex"`
	City      string `json:"city"`
}

type updatePreferencesRequest struct {
	MinAge int16  `json:"min_age"`
	MaxAge int16  `json:"max_age"`
	Sex    string `json:"sex"`
}

type setSleepRequest struct {
	IsSleeping bool `json:"is_sleeping"`
}
