package domain

import "time"

// DiscoveryCandidate is an internal read projection. CreatedAt is users.created_at
// and, together with Profile.UserID, forms its pagination position. Photo keys
// must be converted to URLs before returning an application/API result.
type DiscoveryCandidate struct {
	Profile   Profile
	Photos    []Photo
	CreatedAt time.Time
}

// MatchWithPartner is an internal projection scoped to the requesting participant.
// Partner is the other user; Photo is nil if that user has no photos anymore.
// A closed/sleeping partner remains visible in the owner's match history.
type MatchWithPartner struct {
	Match   Match
	Partner Profile
	Photo   *Photo
}
